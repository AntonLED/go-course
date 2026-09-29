//go:build solution

package importer

import (
	"bufio"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "gocourse/modules/04-data/memsql" // регистрирует драйвер "memsql"
)

const usage = "usage: importer -db DSN [-format csv|jsonl] [-dry-run] [-strict] [FILE]\n"

type options struct {
	dsn    string
	format string
	dryRun bool
	strict bool
	file   string // "" или "-" — stdin
}

// usageError — ошибка аргументов (код 2).
type usageError struct{ error }

// Run — точка входа утилиты.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opt, err := parseArgs(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return ExitOK
	}
	if err != nil {
		fmt.Fprintf(stderr, "importer: %v\n%s", err, usage)
		return ExitUsage
	}
	rep, err := run(context.Background(), opt, stdin)
	var ue usageError
	switch {
	case errors.As(err, &ue):
		fmt.Fprintf(stderr, "importer: %v\n%s", err, usage)
		return ExitUsage
	case err != nil:
		fmt.Fprintf(stderr, "importer: %v\n", err)
		return ExitFailure
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rep); err != nil {
		fmt.Fprintf(stderr, "importer: вывод отчёта: %v\n", err)
		return ExitFailure
	}
	switch {
	case len(rep.Errors) == 0:
		return ExitOK
	case opt.strict:
		fmt.Fprintf(stderr, "importer: %d невалидных записей в режиме -strict, изменения отменены\n", len(rep.Errors))
		return ExitFailure
	default:
		return ExitPartial
	}
}

func parseArgs(args []string, stderr io.Writer) (options, error) {
	var opt options
	fs := flag.NewFlagSet("importer", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opt.dsn, "db", "", "DSN базы данных (обязательно)")
	fs.StringVar(&opt.format, "format", "", "csv или jsonl (по умолчанию — по расширению файла)")
	fs.BoolVar(&opt.dryRun, "dry-run", false, "проверить и посчитать, но ничего не записывать")
	fs.BoolVar(&opt.strict, "strict", false, "при любой невалидной записи ничего не записывать")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(stderr)
			fmt.Fprint(stderr, usage)
			fs.PrintDefaults()
		}
		return opt, err
	}
	if opt.dsn == "" {
		return opt, errors.New("не указан -db")
	}
	switch fs.NArg() {
	case 0:
	case 1:
		opt.file = fs.Arg(0)
	default:
		return opt, fmt.Errorf("ожидается не больше одного файла, получено %d", fs.NArg())
	}
	if opt.format == "" && opt.file != "" && opt.file != "-" {
		name := strings.TrimSuffix(strings.ToLower(opt.file), ".gz")
		switch filepath.Ext(name) {
		case ".csv":
			opt.format = "csv"
		case ".jsonl", ".ndjson":
			opt.format = "jsonl"
		}
	}
	switch opt.format {
	case "csv", "jsonl":
	case "":
		return opt, errors.New("не удалось определить формат: укажите -format")
	default:
		return opt, fmt.Errorf("неизвестный формат %q (csv|jsonl)", opt.format)
	}
	return opt, nil
}

// openInput открывает файл или stdin и прозрачно распаковывает gzip,
// определяя его по магическим байтам 1f 8b (Peek не «съедает» данные).
func openInput(path string, stdin io.Reader) (io.Reader, func() error, error) {
	var src io.Reader = stdin
	closeFn := func() error { return nil }
	if path != "" && path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return nil, nil, err
		}
		src, closeFn = f, f.Close
	}
	br := bufio.NewReader(src)
	magic, _ := br.Peek(2)
	if len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		zr, err := gzip.NewReader(br)
		if err != nil {
			closeFn()
			return nil, nil, fmt.Errorf("gzip: %w", err)
		}
		return zr, func() error { zr.Close(); return closeFn() }, nil
	}
	return br, closeFn, nil
}

// rawRecord — запись до валидации: строки как во входных данных.
type rawRecord struct {
	SKU, Name, Price, Qty string
	Category              *string
}

// emitFunc получает очередную запись (или ошибку разбора этой записи).
type emitFunc func(line int, rec rawRecord, recErr error) error

func run(ctx context.Context, opt options, stdin io.Reader) (Report, error) {
	in, closeIn, err := openInput(opt.file, stdin)
	if err != nil {
		return Report{}, err
	}
	defer closeIn()

	db, err := sql.Open("memsql", opt.dsn)
	if err != nil {
		return Report{}, err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, Schema); err != nil {
		return Report{}, fmt.Errorf("миграция: %w", err)
	}

	// Весь импорт — одна транзакция: либо всё, либо ничего.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Report{}, err
	}
	defer tx.Rollback() // no-op после Commit
	repo := productRepo{tx: tx}

	rep := Report{DryRun: opt.dryRun, Errors: []RecordError{}}
	seen := map[string]int{} // sku → строка первого вхождения
	emit := func(line int, raw rawRecord, recErr error) error {
		rep.Total++
		fail := func(err error) error {
			rep.Errors = append(rep.Errors, RecordError{Line: line, SKU: strings.TrimSpace(raw.SKU), Error: err.Error()})
			return nil // ошибка записи не фатальна — идём дальше
		}
		if recErr != nil {
			return fail(recErr)
		}
		p, err := validate(raw)
		if err != nil {
			return fail(err)
		}
		if first, dup := seen[p.SKU]; dup {
			return fail(fmt.Errorf("sku: дубликат (впервые в строке %d)", first))
		}
		seen[p.SKU] = line
		inserted, err := repo.Upsert(ctx, p)
		if err != nil {
			return fmt.Errorf("строка %d: запись в БД: %w", line, err) // фатально
		}
		if inserted {
			rep.Inserted++
		} else {
			rep.Updated++
		}
		return nil
	}

	switch opt.format {
	case "csv":
		err = readCSV(in, emit)
	default:
		err = readJSONL(in, emit)
	}
	if err != nil {
		return Report{}, err
	}
	rep.Skipped = len(rep.Errors)

	switch {
	case opt.strict && len(rep.Errors) > 0:
		rep.Inserted, rep.Updated = 0, 0 // откатываем (defer tx.Rollback)
	case opt.dryRun:
		// ничего не коммитим: отчёт показывает, что было бы
	default:
		if err := tx.Commit(); err != nil {
			return Report{}, fmt.Errorf("commit: %w", err)
		}
	}
	return rep, nil
}

// --- чтение форматов ---

var requiredCols = []string{"sku", "name", "price", "qty"}

func readCSV(r io.Reader, emit emitFunc) error {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	header, err := cr.Read()
	if errors.Is(err, io.EOF) {
		return errors.New("csv: пустой ввод, нет заголовка")
	}
	if err != nil {
		return fmt.Errorf("csv: заголовок: %w", err)
	}
	col := map[string]int{}
	for i, h := range header {
		if i == 0 {
			h = strings.TrimPrefix(h, "\ufeff") // BOM от Excel
		}
		h = strings.ToLower(strings.TrimSpace(h))
		if _, dup := col[h]; dup {
			return fmt.Errorf("csv: колонка %q повторяется в заголовке", h)
		}
		col[h] = i
	}
	for _, c := range requiredCols {
		if _, ok := col[c]; !ok {
			return fmt.Errorf("csv: в заголовке нет обязательной колонки %q", c)
		}
	}
	catIdx, hasCat := col["category"]

	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		// Строку считаем по позиции первого поля: с многострочными полями
		// в кавычках номер записи и номер строки файла расходятся.
		line := 0
		if len(rec) > 0 {
			line, _ = cr.FieldPos(0)
		}
		if errors.Is(err, csv.ErrFieldCount) {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				line = pe.StartLine
			}
			if err := emit(line, rawRecord{SKU: get(rec, col["sku"])}, errors.New("неверное число полей")); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("csv: %w", err)
		}
		raw := rawRecord{
			SKU:   rec[col["sku"]],
			Name:  rec[col["name"]],
			Price: rec[col["price"]],
			Qty:   rec[col["qty"]],
		}
		if hasCat {
			c := rec[catIdx]
			raw.Category = &c
		}
		if err := emit(line, raw, nil); err != nil {
			return err
		}
	}
}

func get(rec []string, i int) string {
	if i < len(rec) {
		return rec[i]
	}
	return ""
}

// jsonRecord — строка JSONL. json.Number принимает и число 12.34, и строку
// "12.34" и сохраняет текст как есть — без округлений float64.
type jsonRecord struct {
	SKU      *string     `json:"sku"`
	Name     *string     `json:"name"`
	Price    json.Number `json:"price"`
	Qty      json.Number `json:"qty"`
	Category *string     `json:"category"`
}

func readJSONL(r io.Reader, emit emitFunc) error {
	br := bufio.NewReader(r) // без лимита длины строки, в отличие от Scanner
	for line := 1; ; line++ {
		text, err := br.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if strings.TrimSpace(text) != "" {
			var jr jsonRecord
			dec := json.NewDecoder(strings.NewReader(text))
			dec.DisallowUnknownFields()
			recErr := dec.Decode(&jr)
			// После объекта в строке не должно быть ничего, кроме пробелов.
			// dec.More() здесь не годится: для хвоста "}" или "]" он вернёт false.
			if recErr == nil {
				if _, err := dec.Token(); err != io.EOF {
					recErr = errors.New("лишние данные после JSON-объекта")
				}
			}
			raw := rawRecord{Price: jr.Price.String(), Qty: jr.Qty.String(), Category: jr.Category}
			if jr.SKU != nil {
				raw.SKU = *jr.SKU
			}
			if jr.Name != nil {
				raw.Name = *jr.Name
			}
			if recErr != nil {
				recErr = fmt.Errorf("json: %w", recErr)
			}
			if err := emit(line, raw, recErr); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
	}
}

// --- валидация ---

func validate(raw rawRecord) (Product, error) {
	p := Product{SKU: strings.TrimSpace(raw.SKU), Name: strings.TrimSpace(raw.Name)}
	if p.SKU == "" {
		return p, errors.New("sku: обязательное поле")
	}
	if len(p.SKU) > 32 {
		return p, errors.New("sku: длиннее 32 символов")
	}
	for _, c := range p.SKU {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return p, fmt.Errorf("sku: недопустимый символ %q", c)
		}
	}
	if p.Name == "" {
		return p, errors.New("name: обязательное поле")
	}
	var err error
	if p.PriceCents, err = parsePrice(strings.TrimSpace(raw.Price)); err != nil {
		return p, fmt.Errorf("price: %w", err)
	}
	q := strings.TrimSpace(raw.Qty)
	if p.Qty, err = strconv.ParseInt(q, 10, 64); err != nil || p.Qty < 0 {
		return p, fmt.Errorf("qty: ожидалось целое >= 0, получено %q", q)
	}
	if raw.Category != nil {
		if c := strings.TrimSpace(*raw.Category); c != "" {
			p.Category = &c
		}
	}
	return p, nil
}

// parsePrice разбирает "12", "12.5", "12.50" в копейки/центы без float:
// float64("0.29")*100 == 28.999999999999996.
func parsePrice(s string) (int64, error) {
	intPart, frac, hasDot := strings.Cut(s, ".")
	if intPart == "" || (hasDot && frac == "") || len(frac) > 2 {
		return 0, fmt.Errorf("ожидалась сумма вида 12.34, получено %q", s)
	}
	for _, c := range intPart + frac {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("ожидалась неотрицательная сумма вида 12.34, получено %q", s)
		}
	}
	for len(frac) < 2 {
		frac += "0"
	}
	units, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil || units > (1<<62)/100 {
		return 0, fmt.Errorf("слишком большая сумма %q", s)
	}
	cents, _ := strconv.ParseInt(frac, 10, 64)
	return units*100 + cents, nil
}

// --- репозиторий ---

// productRepo работает внутри транзакции. В реальном PostgreSQL upsert
// делается одним запросом: INSERT ... ON CONFLICT (sku) DO UPDATE ...
// (и RETURNING (xmax = 0) AS inserted, чтобы узнать, была ли вставка).
type productRepo struct {
	tx *sql.Tx
}

func (r productRepo) Upsert(ctx context.Context, p Product) (inserted bool, err error) {
	res, err := r.tx.ExecContext(ctx,
		`UPDATE products SET name = ?, price_cents = ?, qty = ?, category = ? WHERE sku = ?`,
		p.Name, p.PriceCents, p.Qty, p.Category, p.SKU)
	if err != nil {
		return false, err
	}
	if n, err := res.RowsAffected(); err != nil || n > 0 {
		return false, err
	}
	_, err = r.tx.ExecContext(ctx,
		`INSERT INTO products (sku, name, price_cents, qty, category) VALUES (?, ?, ?, ?, ?)`,
		p.SKU, p.Name, p.PriceCents, p.Qty, p.Category)
	return err == nil, err
}
