package importer

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gocourse/modules/04-data/memsql"
)

type cliResult struct {
	code           int
	stdout, stderr string
	report         Report
}

func runImporter(t *testing.T, stdin string, args ...string) cliResult {
	t.Helper()
	var out, errb bytes.Buffer
	code := Run(args, strings.NewReader(stdin), &out, &errb)
	r := cliResult{code: code, stdout: out.String(), stderr: errb.String()}
	if out.Len() > 0 {
		if err := json.Unmarshal(out.Bytes(), &r.report); err != nil {
			t.Fatalf("stdout не является JSON-отчётом: %v\n%s", err, out.String())
		}
	}
	return r
}

func testDSN(t *testing.T) string {
	dsn := memsql.NewDSN(t.Name())
	t.Cleanup(func() { memsql.Drop(dsn) })
	return dsn
}

type dbRow struct {
	SKU, Name  string
	Price, Qty int64
	Category   sql.NullString
}

// dump читает таблицу products тем же DSN, что использовал Run.
func dumpProducts(t *testing.T, dsn string) []dbRow {
	t.Helper()
	db, err := sql.Open("memsql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT sku, name, price_cents, qty, category FROM products ORDER BY sku`)
	if err != nil {
		return nil // таблицы нет
	}
	defer rows.Close()
	var out []dbRow
	for rows.Next() {
		var r dbRow
		if err := rows.Scan(&r.SKU, &r.Name, &r.Price, &r.Qty, &r.Category); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func nullStr(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

const goodCSV = "\ufeffSKU,Name,Price,Qty,Category,Comment\n" +
	"A-1,Ручка,12.5,10,office,x\n" +
	"B_2, \"  Тетрадь, 48 л. \",0.29,0,,\n" +
	"c3,Клей,100,3,glue,\n"

func TestCSVHappyPath(t *testing.T) {
	dsn := testDSN(t)
	r := runImporter(t, goodCSV, "-db", dsn, "-format", "csv")
	if r.code != ExitOK {
		t.Fatalf("код %d, stderr: %s", r.code, r.stderr)
	}
	want := Report{Total: 3, Inserted: 3, Errors: []RecordError{}}
	if !reflect.DeepEqual(r.report, want) {
		t.Errorf("отчёт = %+v, ожидалось %+v", r.report, want)
	}
	if !strings.Contains(r.stdout, `"errors": []`) {
		t.Errorf("errors должен быть [], а не null; stdout:\n%s", r.stdout)
	}
	got := dumpProducts(t, dsn)
	wantRows := []dbRow{
		{"A-1", "Ручка", 1250, 10, nullStr("office")},
		{"B_2", "Тетрадь, 48 л.", 29, 0, sql.NullString{}},
		{"c3", "Клей", 10000, 3, nullStr("glue")},
	}
	if !reflect.DeepEqual(got, wantRows) {
		t.Errorf("в базе:\n %+v\nожидалось\n %+v", got, wantRows)
	}
}

func TestJSONLHappyPathAndUpsert(t *testing.T) {
	dsn := testDSN(t)
	in := `{"sku":"A-1","name":"Ручка","price":12.34,"qty":1}` + "\n" +
		"\n" +
		`{"sku":"B-2","name":"Карандаш","price":"0.10","qty":"7","category":"office"}` + "\n" +
		`{"sku":"C-3","name":"Ластик","price":1,"qty":0,"category":null}` // без \n в конце
	r := runImporter(t, in, "-db", dsn, "-format=jsonl")
	if r.code != ExitOK || r.report.Inserted != 3 || r.report.Total != 3 {
		t.Fatalf("код %d, отчёт %+v, stderr %s", r.code, r.report, r.stderr)
	}
	// Повторный импорт: A-1 обновляется, D-4 добавляется.
	in2 := `{"sku":"A-1","name":"Ручка синяя","price":15,"qty":2}` + "\n" + `{"sku":"D-4","name":"Скрепки","price":0.5,"qty":100}`
	r = runImporter(t, in2, "-db", dsn, "-format", "jsonl")
	if r.code != ExitOK || r.report.Inserted != 1 || r.report.Updated != 1 {
		t.Fatalf("upsert: код %d, отчёт %+v", r.code, r.report)
	}
	got := dumpProducts(t, dsn)
	want := []dbRow{
		{"A-1", "Ручка синяя", 1500, 2, sql.NullString{}},
		{"B-2", "Карандаш", 10, 7, nullStr("office")},
		{"C-3", "Ластик", 100, 0, sql.NullString{}},
		{"D-4", "Скрепки", 50, 100, sql.NullString{}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("в базе:\n %+v\nожидалось\n %+v", got, want)
	}
}

const mixedJSONL = `{"sku":"ok-1","name":"Хорошая","price":1,"qty":1}
{"sku":"","name":"Без SKU","price":1,"qty":1}
{"sku":"bad sku","name":"Пробел","price":1,"qty":1}
{"sku":"p1","name":"Цена","price":1.234,"qty":1}
{"sku":"p2","name":"Цена","price":-1,"qty":1}
{"sku":"q1","name":"Кол-во","price":1,"qty":1.5}
{"sku":"n1","name":"  ","price":1,"qty":1}
{"sku":"u1","name":"Лишнее поле","price":1,"qty":1,"color":"red"}
{"sku":"ok-1","name":"Дубль","price":2,"qty":2}
не json
{"sku":"two","name":"x","price":1,"qty":1} {"sku":"objs"}
{"sku":"ok-2","name":"Вторая хорошая","price":"3.5","qty":0}
`

func TestPartialAndErrors(t *testing.T) {
	dsn := testDSN(t)
	r := runImporter(t, mixedJSONL, "-db", dsn, "-format", "jsonl")
	if r.code != ExitPartial {
		t.Fatalf("код %d, ожидалось %d (ExitPartial); stderr: %s", r.code, ExitPartial, r.stderr)
	}
	rep := r.report
	if rep.Total != 12 || rep.Inserted != 2 || rep.Skipped != 10 || len(rep.Errors) != 10 {
		t.Errorf("отчёт = total %d, inserted %d, skipped %d, errors %d; ожидалось 12, 2, 10, 10",
			rep.Total, rep.Inserted, rep.Skipped, len(rep.Errors))
	}
	var lines []int
	for _, e := range rep.Errors {
		lines = append(lines, e.Line)
		if e.Error == "" {
			t.Errorf("пустой текст ошибки в строке %d", e.Line)
		}
	}
	if !reflect.DeepEqual(lines, []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 11}) {
		t.Errorf("номера строк с ошибками = %v", lines)
	}
	if len(rep.Errors) > 7 && rep.Errors[7].SKU != "ok-1" {
		t.Errorf("для дубликата SKU должен быть в отчёте: %+v", rep.Errors[7])
	}
	if got := dumpProducts(t, dsn); len(got) != 2 || got[0].Name != "Хорошая" || got[1].Price != 350 {
		t.Errorf("в базе: %+v", got)
	}
}

func TestJSONLTrailingGarbage(t *testing.T) {
	dsn := testDSN(t)
	in := `{"sku":"t1","name":"x","price":1,"qty":1}}` + "\n" +
		`{"sku":"t2","name":"y","price":1,"qty":1}   ` + "\n"
	r := runImporter(t, in, "-db", dsn, "-format", "jsonl")
	if r.code != ExitPartial || r.report.Inserted != 1 || len(r.report.Errors) != 1 || r.report.Errors[0].Line != 1 {
		t.Errorf("лишняя '}' после объекта — невалидная запись в строке 1: код %d, отчёт %+v", r.code, r.report)
	}
}

func TestStrictAndDryRun(t *testing.T) {
	dsn := testDSN(t)
	r := runImporter(t, mixedJSONL, "-db", dsn, "-format", "jsonl", "-strict")
	if r.code != ExitFailure {
		t.Errorf("-strict с ошибками: код %d, ожидалось %d", r.code, ExitFailure)
	}
	if r.report.Inserted != 0 || r.report.Skipped != 10 || r.stderr == "" {
		t.Errorf("-strict: отчёт %+v, stderr %q", r.report, r.stderr)
	}
	if got := dumpProducts(t, dsn); len(got) != 0 {
		t.Errorf("-strict: в базе не должно быть записей, есть %+v", got)
	}

	r = runImporter(t, goodCSV, "-db", dsn, "-format", "csv", "-dry-run")
	if r.code != ExitOK || !r.report.DryRun || r.report.Inserted != 3 {
		t.Errorf("-dry-run: код %d, отчёт %+v", r.code, r.report)
	}
	if got := dumpProducts(t, dsn); len(got) != 0 {
		t.Errorf("-dry-run: в базе не должно быть записей, есть %+v", got)
	}

	r = runImporter(t, goodCSV, "-strict", "-db", dsn, "-format", "csv")
	if r.code != ExitOK || len(dumpProducts(t, dsn)) != 3 {
		t.Errorf("-strict без ошибок должен записать всё: код %d", r.code)
	}
}

func TestCSVDetails(t *testing.T) {
	dsn := testDSN(t)
	in := "qty,price,sku,name\n" + // произвольный порядок колонок, без category
		"1,1,a,\"Многострочное\nназвание\"\n" +
		"2,2,b\n" + // мало полей
		"3,3.999,c,Цена\n" +
		"4,4,d,Норм\n"
	r := runImporter(t, in, "-db", dsn, "-format", "csv")
	if r.code != ExitPartial {
		t.Fatalf("код %d; stderr %s", r.code, r.stderr)
	}
	var lines []int
	for _, e := range r.report.Errors {
		lines = append(lines, e.Line)
	}
	if !reflect.DeepEqual(lines, []int{4, 5}) {
		t.Errorf("строки ошибок = %v, ожидалось [4 5] (многострочное поле занимает строки 2–3)", lines)
	}
	got := dumpProducts(t, dsn)
	if len(got) != 2 || got[0].Name != "Многострочное\nназвание" || got[1].SKU != "d" {
		t.Errorf("в базе: %+v", got)
	}

	for name, bad := range map[string]string{
		"пусто":         "",
		"нет колонки":   "sku,name,price\na,b,1\n",
		"дубль колонки": "sku,name,price,qty,sku\n",
		"битые кавычки": "sku,name,price,qty\na,\"b,1,1\n",
	} {
		r := runImporter(t, bad, "-db", testDSN(t), "-format", "csv")
		if r.code != ExitFailure || r.stdout != "" || r.stderr == "" {
			t.Errorf("%s: код %d, stdout %q, stderr %q; ожидалось 1, пустой stdout, ошибка в stderr", name, r.code, r.stdout, r.stderr)
		}
	}
}

func TestFilesAndGzip(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "items.CSV")
	os.WriteFile(csvPath, []byte(goodCSV), 0o644)

	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write([]byte(`{"sku":"z","name":"Из архива","price":9.99,"qty":1}` + "\n"))
	zw.Close()
	gzPath := filepath.Join(dir, "items.jsonl.gz")
	os.WriteFile(gzPath, gz.Bytes(), 0o644)

	dsn := testDSN(t)
	if r := runImporter(t, "", "-db", dsn, csvPath); r.code != ExitOK || r.report.Inserted != 3 {
		t.Errorf("CSV-файл (формат по расширению): код %d, отчёт %+v, stderr %s", r.code, r.report, r.stderr)
	}
	if r := runImporter(t, "", "-db", dsn, gzPath); r.code != ExitOK || r.report.Inserted != 1 {
		t.Errorf("gzip-файл: код %d, отчёт %+v, stderr %s", r.code, r.report, r.stderr)
	}
	// gzip в stdin определяется по магическим байтам.
	if r := runImporter(t, gz.String(), "-db", testDSN(t), "-format", "jsonl", "-"); r.code != ExitOK || r.report.Inserted != 1 {
		t.Errorf("gzip из stdin: код %d, отчёт %+v, stderr %s", r.code, r.report, r.stderr)
	}
	if r := runImporter(t, "", "-db", dsn, filepath.Join(dir, "missing.csv")); r.code != ExitFailure || r.stderr == "" {
		t.Errorf("несуществующий файл: код %d", r.code)
	}
}

func TestUsage(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"без -db", []string{"-format", "csv"}},
		{"нет формата у stdin", []string{"-db", "x"}},
		{"неизвестный формат", []string{"-db", "x", "-format", "xml"}},
		{"неизвестное расширение", []string{"-db", "x", "data.txt"}},
		{"два файла", []string{"-db", "x", "a.csv", "b.csv"}},
		{"неизвестный флаг", []string{"-db", "x", "-format", "csv", "-force"}},
	}
	for _, tt := range tests {
		r := runImporter(t, "", tt.args...)
		if r.code != ExitUsage || r.stdout != "" || !strings.Contains(r.stderr, "usage") {
			t.Errorf("%s: код %d, stdout %q, stderr %q; ожидалось 2, пустой stdout, usage в stderr", tt.name, r.code, r.stdout, r.stderr)
		}
	}
	if r := runImporter(t, "", "-h"); r.code != ExitOK || r.stdout != "" || !strings.Contains(r.stderr, "-db") {
		t.Errorf("-h: код %d, stderr %q", r.code, r.stderr)
	}
}

func TestPriceParsing(t *testing.T) {
	cases := map[string]int64{"0": 0, "0.29": 29, "0.1": 10, "12": 1200, "12.50": 1250, "007.07": 707, "99999999.99": 9999999999}
	bad := []string{"", ".5", "5.", "1.234", "-1", "+1", "1e3", "1,5", "abc", " ", "1.2.3", "99999999999999999999"}
	var in strings.Builder
	in.WriteString("sku,name,price,qty\n")
	i := 0
	for p := range cases {
		i++
		in.WriteString("ok" + string(rune('a'+i)) + ",n," + "\"" + p + "\",1\n")
	}
	for j, p := range bad {
		in.WriteString("bad" + string(rune('a'+j)) + ",n,\"" + p + "\",1\n")
	}
	dsn := testDSN(t)
	r := runImporter(t, in.String(), "-db", dsn, "-format", "csv")
	if r.report.Inserted != len(cases) || r.report.Skipped != len(bad) {
		t.Errorf("inserted %d, skipped %d; ожидалось %d и %d; ошибки: %+v", r.report.Inserted, r.report.Skipped, len(cases), len(bad), r.report.Errors)
	}
	got := map[int64]bool{}
	for _, dbRow := range dumpProducts(t, dsn) {
		got[dbRow.Price] = true
	}
	for p, want := range cases {
		if !got[want] {
			t.Errorf("цена %q должна дать %d копеек (без float!)", p, want)
		}
	}
}
