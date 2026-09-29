// Команда check — сводка прогресса по задачам курса.
//
//	go run ./cmd/check              # все модули
//	go run ./cmd/check 03           # только модуль 03
//	go run ./cmd/check 03 05        # модули 03 и 05
//	go run ./cmd/check -race 03     # с детектором гонок
//	go run ./cmd/check -solution    # прогнать эталонные решения (проверка самого курса)
//	go run ./cmd/check -v 02        # показать упавшие подтесты
//
// Под капотом: go test -json -count=1 ./modules/NN-*/tasks/...
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type event struct {
	Action  string
	Package string
	Test    string
	Output  string
}

type pkgResult struct {
	status        string // pass | fail | skip | build
	passed, total int
	failed        []string
	output        strings.Builder
}

func main() {
	race := flag.Bool("race", false, "запускать с -race")
	solution := flag.Bool("solution", false, "проверять эталонные решения (-tags solution)")
	verbose := flag.Bool("v", false, "показывать упавшие подтесты и ошибки сборки")
	flag.Parse()

	mods, _ := filepath.Glob("modules/[0-9][0-9]-*")
	sort.Strings(mods)
	if len(mods) == 0 {
		fmt.Fprintln(os.Stderr, "запускай из корня курса (там, где go.mod)")
		os.Exit(2)
	}
	if flag.NArg() > 0 {
		want := map[string]bool{}
		for _, a := range flag.Args() {
			if len(a) == 1 {
				a = "0" + a
			}
			want[a] = true
		}
		var sel []string
		for _, m := range mods {
			if want[filepath.Base(m)[:2]] {
				sel = append(sel, m)
			}
		}
		if len(sel) == 0 {
			fmt.Fprintln(os.Stderr, "нет таких модулей:", flag.Args())
			os.Exit(2)
		}
		mods = sel
	}

	args := []string{"test", "-json", "-count=1", "-timeout=120s"}
	if *race {
		args = append(args, "-race")
	}
	if *solution {
		args = append(args, "-tags", "solution")
	}
	for _, m := range mods {
		args = append(args, "./"+m+"/tasks/...")
	}

	cmd := exec.Command("go", args...)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	res := map[string]*pkgResult{}
	get := func(p string) *pkgResult {
		r, ok := res[p]
		if !ok {
			r = &pkgResult{}
			res[p] = r
		}
		return r
	}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	fmt.Fprint(os.Stderr, "Гоняю тесты")
	for sc.Scan() {
		var e event
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.Package == "" {
			continue
		}
		r := get(e.Package)
		switch {
		case e.Test == "" && e.Action == "output":
			r.output.WriteString(e.Output)
		case e.Test == "" && (e.Action == "pass" || e.Action == "fail" || e.Action == "skip"):
			if r.status == "" {
				r.status = e.Action
			}
			fmt.Fprint(os.Stderr, ".")
		case e.Test != "" && !strings.Contains(e.Test, "/"):
			switch e.Action {
			case "pass":
				r.passed++
				r.total++
			case "fail":
				r.total++
				r.failed = append(r.failed, e.Test)
			}
		case e.Test != "" && e.Action == "fail":
			r.failed = append(r.failed, e.Test)
		}
	}
	cmd.Wait()
	fmt.Fprintln(os.Stderr)

	pkgs := make([]string, 0, len(res))
	for p := range res {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)

	done, all := 0, 0
	cur := ""
	for _, p := range pkgs {
		r := res[p]
		rel := strings.TrimPrefix(p, "gocourse/modules/")
		mod, task, _ := strings.Cut(rel, "/tasks/")
		if task == "" || strings.Contains(task, "/") {
			continue // вспомогательные пакеты (cmd и т.п.)
		}
		if r.status == "skip" && r.total == 0 {
			continue // пакет без тестов
		}
		if mod != cur {
			cur = mod
			fmt.Printf("\n== %s\n", mod)
		}
		all++
		mark := "❌"
		note := fmt.Sprintf("%d/%d тестов", r.passed, r.total)
		if r.status == "pass" {
			mark = "✅"
			done++
		} else if r.total == 0 {
			note = "не собирается или паника"
		}
		fmt.Printf("  %s %-28s %s\n", mark, task, note)
		if *verbose && r.status != "pass" {
			for _, f := range r.failed {
				fmt.Printf("       ✗ %s\n", f)
			}
			if r.total == 0 {
				for _, line := range strings.Split(strings.TrimSpace(r.output.String()), "\n") {
					fmt.Printf("       │ %s\n", line)
				}
			}
		}
	}
	fmt.Printf("\nИтого решено: %d из %d задач\n", done, all)
	if done != all {
		os.Exit(1)
	}
}
