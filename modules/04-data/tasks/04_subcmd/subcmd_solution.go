//go:build solution

package subcmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"unicode"
)

// usageError — ошибка использования (код 2).
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

// app хранит общее для всех команд состояние.
type app struct {
	stdin          io.Reader
	stdout, stderr io.Writer
	json           bool
}

// Run — точка входа, main сводится к os.Exit(Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)).
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	a := &app{stdin: stdin, stdout: stdout, stderr: stderr}

	global := flag.NewFlagSet("textkit", flag.ContinueOnError)
	global.SetOutput(io.Discard) // справку печатаем сами
	global.BoolVar(&a.json, "json", false, "вывод в JSON")
	if err := global.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stderr, Usage)
			return ExitOK
		}
		fmt.Fprintf(stderr, "textkit: %v\n%s", err, Usage)
		return ExitUsage
	}
	rest := global.Args()
	if len(rest) == 0 {
		fmt.Fprintf(stderr, "textkit: не указана команда\n%s", Usage)
		return ExitUsage
	}

	var cmd func([]string) error
	switch name := rest[0]; name {
	case "count":
		cmd = a.count
	case "repeat":
		cmd = a.repeat
	case "upper":
		cmd = a.upper
	case "version":
		cmd = a.version
	default:
		fmt.Fprintf(stderr, "textkit: неизвестная команда %q\n%s", name, Usage)
		return ExitUsage
	}

	err := cmd(rest[1:])
	var ue usageError
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, flag.ErrHelp):
		return ExitOK // справку команды уже напечатал FlagSet
	case errors.As(err, &ue):
		fmt.Fprintf(stderr, "textkit %s: %v\n", rest[0], err)
		return ExitUsage
	default:
		fmt.Fprintf(stderr, "textkit %s: %v\n", rest[0], err)
		return ExitFailure
	}
}

// newFlagSet создаёт FlagSet команды: ошибки разбора и -h печатаются в stderr.
func (a *app) newFlagSet(name, synopsis string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	fs.Usage = func() {
		fmt.Fprintf(a.stderr, "usage: textkit %s %s\n", name, synopsis)
		fs.PrintDefaults()
	}
	return fs
}

// parse разбирает флаги команды, превращая ошибки разбора в usageError.
func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return usageError{err.Error()}
	}
	return nil
}

func (a *app) count(args []string) error {
	fs := a.newFlagSet("count", "[-lines] [-words] [-bytes]")
	lines := fs.Bool("lines", false, "число строк ('\\n')")
	words := fs.Bool("words", false, "число слов")
	bytes := fs.Bool("bytes", false, "число байт")
	if err := parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usageError{"лишние аргументы: " + strings.Join(fs.Args(), " ")}
	}
	if !*lines && !*words && !*bytes {
		*lines, *words, *bytes = true, true, true
	}

	// Потоковый подсчёт: не читаем stdin целиком в память.
	var nl, nw, nb int
	inWord := false
	br := bufio.NewReader(a.stdin)
	for {
		r, size, err := br.ReadRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("чтение stdin: %w", err)
		}
		nb += size
		if r == '\n' {
			nl++
		}
		if unicode.IsSpace(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			nw++
		}
	}

	type kv struct {
		key string
		val int
		on  bool
	}
	items := []kv{{"lines", nl, *lines}, {"words", nw, *words}, {"bytes", nb, *bytes}}
	if a.json {
		m := map[string]int{}
		for _, it := range items {
			if it.on {
				m[it.key] = it.val
			}
		}
		return json.NewEncoder(a.stdout).Encode(m)
	}
	var parts []string
	for _, it := range items {
		if it.on {
			parts = append(parts, fmt.Sprintf("%s=%d", it.key, it.val))
		}
	}
	_, err := fmt.Fprintln(a.stdout, strings.Join(parts, " "))
	return err
}

func (a *app) repeat(args []string) error {
	fs := a.newFlagSet("repeat", "[-n N] [-sep S] TEXT...")
	n := fs.Int("n", 2, "сколько раз повторить (>= 0)")
	sep := fs.String("sep", " ", "разделитель")
	if err := parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return usageError{"не указан TEXT"}
	}
	if *n < 0 {
		return usageError{fmt.Sprintf("-n должен быть >= 0, получено %d", *n)}
	}
	text := strings.Join(fs.Args(), " ")
	out := make([]string, *n) // не nil даже при n == 0 → JSON "[]", а не "null"
	for i := range out {
		out[i] = text
	}
	if a.json {
		return json.NewEncoder(a.stdout).Encode(out)
	}
	_, err := fmt.Fprintln(a.stdout, strings.Join(out, *sep))
	return err
}

func (a *app) upper(args []string) error {
	fs := a.newFlagSet("upper", "")
	if err := parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usageError{"команда не принимает аргументов"}
	}
	br := bufio.NewReader(a.stdin)
	bw := bufio.NewWriter(a.stdout)
	for {
		// Построчно: ReadString не разрывает многобайтовые руны.
		line, err := br.ReadString('\n')
		if _, werr := bw.WriteString(strings.ToUpper(line)); werr != nil {
			return werr
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			bw.Flush()
			return fmt.Errorf("чтение stdin: %w", err)
		}
	}
	return bw.Flush() // без Flush хвост вывода потеряется
}

func (a *app) version(args []string) error {
	fs := a.newFlagSet("version", "")
	if err := parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usageError{"команда не принимает аргументов"}
	}
	if a.json {
		return json.NewEncoder(a.stdout).Encode(map[string]string{"version": Version})
	}
	_, err := fmt.Fprintf(a.stdout, "textkit %s\n", Version)
	return err
}
