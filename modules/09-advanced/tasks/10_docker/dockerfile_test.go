package dockerapp

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// ---------- мини-парсер Dockerfile ----------

type instruction struct {
	cmd  string // FROM, RUN, COPY... (в верхнем регистре)
	args string
	line int
}

type stage struct {
	image string // после подстановки ARG
	name  string // AS <name>
	insts []instruction
}

func parseDockerfile(src string) (globalArgs map[string]string, stages []*stage) {
	globalArgs = map[string]string{}
	lines := strings.Split(src, "\n")
	var buf strings.Builder
	start := 0
	flush := func(lineNo int) {
		text := strings.TrimSpace(buf.String())
		buf.Reset()
		if text == "" {
			return
		}
		cmd, args, _ := strings.Cut(text, " ")
		inst := instruction{cmd: strings.ToUpper(cmd), args: strings.TrimSpace(args), line: lineNo}
		if inst.cmd == "FROM" {
			st := &stage{}
			fields := strings.Fields(inst.args)
			var rest []string
			for _, f := range fields {
				if !strings.HasPrefix(f, "--") { // --platform=...
					rest = append(rest, f)
				}
			}
			if len(rest) > 0 {
				st.image = expandArgs(rest[0], globalArgs)
			}
			if len(rest) >= 3 && strings.EqualFold(rest[1], "AS") {
				st.name = rest[2]
			}
			stages = append(stages, st)
			return
		}
		if len(stages) == 0 {
			if inst.cmd == "ARG" {
				k, v, _ := strings.Cut(inst.args, "=")
				globalArgs[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
			}
			return
		}
		cur := stages[len(stages)-1]
		cur.insts = append(cur.insts, inst)
	}
	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "#") {
			continue // комментарии (в т.ч. внутри продолжений строки)
		}
		if buf.Len() == 0 {
			start = i + 1
		}
		if strings.HasSuffix(trimmed, `\`) {
			buf.WriteString(strings.TrimSuffix(trimmed, `\`))
			buf.WriteByte(' ')
			continue
		}
		buf.WriteString(trimmed)
		flush(start)
	}
	flush(start)
	return globalArgs, stages
}

var argRe = regexp.MustCompile(`\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?`)

func expandArgs(s string, args map[string]string) string {
	return argRe.ReplaceAllStringFunc(s, func(m string) string {
		return args[argRe.FindStringSubmatch(m)[1]]
	})
}

func (s *stage) find(cmd string) []instruction {
	var out []instruction
	for _, in := range s.insts {
		if in.cmd == cmd {
			out = append(out, in)
		}
	}
	return out
}

var (
	flagS = regexp.MustCompile(`(^|[\s"'=])-s($|[\s"'])`)
	flagW = regexp.MustCompile(`(^|[\s"'=])-w($|[\s"'])`)
)

// ---------- проверки ----------

func TestDockerfileLint(t *testing.T) {
	data, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("не удалось прочитать %s: %v", dockerfilePath, err)
	}
	_, stages := parseDockerfile(string(data))
	if len(stages) == 0 {
		t.Fatal("в Dockerfile нет ни одного FROM")
	}
	final := stages[len(stages)-1]
	builders := stages[:len(stages)-1]
	stageNames := map[string]bool{}

	t.Run("multi-stage", func(t *testing.T) {
		if len(stages) < 2 {
			t.Errorf("нужна multi-stage сборка (>= 2 FROM), найдено %d", len(stages))
		}
	})

	t.Run("фиксированные теги образов", func(t *testing.T) {
		for _, st := range stages {
			img := st.image
			switch {
			case img == "":
				t.Error("FROM без образа")
			case img == "scratch" || stageNames[img]:
			case strings.Contains(img, "@sha256:"):
			default:
				last := img[strings.LastIndex(img, "/")+1:]
				_, tag, ok := strings.Cut(last, ":")
				if !ok || tag == "" || tag == "latest" {
					t.Errorf("образ %q: нужен фиксированный тег (не latest) или digest", img)
				}
			}
			if st.name != "" {
				stageNames[st.name] = true
			}
		}
	})

	t.Run("флаги go build", func(t *testing.T) {
		var build string
		for _, st := range builders {
			env := ""
			for _, in := range st.insts {
				if in.cmd == "ENV" || in.cmd == "ARG" { // ARG тоже видна в RUN как переменная окружения
					env += " " + in.args
				}
				if in.cmd == "RUN" && strings.Contains(in.args, "go build") {
					build = env + " " + in.args
				}
			}
		}
		if build == "" {
			t.Fatal("в стадии сборки нет RUN ... go build")
		}
		if !regexp.MustCompile(`CGO_ENABLED[= ]0`).MatchString(build) {
			t.Error("нужен CGO_ENABLED=0 (статический бинарник для distroless/scratch)")
		}
		if !strings.Contains(build, "-trimpath") {
			t.Error("нужен флаг -trimpath")
		}
		if !strings.Contains(build, "-ldflags") || !flagS.MatchString(build) || !flagW.MatchString(build) {
			t.Error(`нужны -ldflags "-s -w ..." (без таблицы символов и DWARF)`)
		}
		if !regexp.MustCompile(`-X[= ]+['"]?main\.version=`).MatchString(build) {
			t.Error("версия должна прошиваться через -X main.version=...")
		}
	})

	t.Run("кэш модулей BuildKit", func(t *testing.T) {
		found := false
		for _, st := range builders {
			for _, in := range st.find("RUN") {
				if strings.Contains(in.args, "--mount=type=cache") {
					found = true
				}
			}
		}
		if !found {
			t.Error("используйте RUN --mount=type=cache,target=/go/pkg/mod (и/или /root/.cache/go-build)")
		}
	})

	t.Run("минимальный финальный образ", func(t *testing.T) {
		if final.image != "scratch" && !strings.HasPrefix(final.image, "gcr.io/distroless/") {
			t.Errorf("финальный образ %q: ожидался scratch или gcr.io/distroless/*", final.image)
		}
	})

	t.Run("non-root USER", func(t *testing.T) {
		users := final.find("USER")
		if len(users) == 0 {
			t.Fatal("в финальной стадии нет USER")
		}
		u, _, _ := strings.Cut(users[len(users)-1].args, ":")
		if u == "root" || u == "0" || u == "" {
			t.Errorf("USER %q — запуск от root", users[len(users)-1].args)
		}
	})

	t.Run("EXPOSE", func(t *testing.T) {
		if len(final.find("EXPOSE")) == 0 {
			t.Error("в финальной стадии нет EXPOSE")
		}
	})

	t.Run("COPY в финальной стадии", func(t *testing.T) {
		copies := final.find("COPY")
		if len(copies) == 0 {
			t.Error("финальная стадия должна копировать бинарник: COPY --from=<стадия> ...")
		}
		for _, in := range copies {
			if !strings.Contains(in.args, "--from=") {
				t.Errorf("строка %d: COPY %s — в финальную стадию копируем только артефакты сборки (--from=)", in.line, in.args)
			}
			// Источники — все позиционные аргументы, кроме последнего (назначения).
			// «.» как назначение (COPY --from=build /out/app .) допустимо.
			var pos []string
			for _, f := range strings.Fields(in.args) {
				if !strings.HasPrefix(f, "--") {
					pos = append(pos, f)
				}
			}
			for i, f := range pos {
				if i < len(pos)-1 && (f == "." || f == "./") {
					t.Errorf("строка %d: COPY . . в финальной стадии тащит в образ исходники", in.line)
					break
				}
			}
		}
	})

	t.Run("exec-форма ENTRYPOINT/CMD", func(t *testing.T) {
		var last *instruction
		for i, in := range final.insts {
			if in.cmd == "ENTRYPOINT" || in.cmd == "CMD" {
				last = &final.insts[i]
			}
		}
		if last == nil {
			t.Fatal("нет ENTRYPOINT/CMD в финальной стадии")
		}
		if !strings.HasPrefix(last.args, "[") {
			t.Errorf("%s %s: shell-форма запускает /bin/sh -c, и приложение не получит SIGTERM как PID 1; используйте [\"/server\"]", last.cmd, last.args)
		}
	})

	t.Run("HEALTHCHECK", func(t *testing.T) {
		hc := final.find("HEALTHCHECK")
		if len(hc) == 0 {
			t.Fatal("нет HEALTHCHECK в финальной стадии")
		}
		if !regexp.MustCompile(`CMD\s+\[`).MatchString(hc[0].args) {
			t.Errorf("HEALTHCHECK %s: в distroless/scratch нет shell — нужна exec-форма CMD [...]", hc[0].args)
		}
	})

	t.Run(".dockerignore", func(t *testing.T) {
		ignore, err := os.ReadFile(dockerfilePath + ".dockerignore")
		if err != nil {
			t.Fatalf("нужен файл %s.dockerignore рядом с Dockerfile: %v", dockerfilePath, err)
		}
		if !regexp.MustCompile(`(?m)^(/|\*\*/)?\.git(/|\*)?\s*$`).Match(ignore) {
			t.Error(".dockerignore должен исключать .git")
		}
	})
}

// Проверяем сам линтер на заведомо плохом файле, чтобы он не был «всегда зелёным».
func TestLinterParser(t *testing.T) {
	src := "ARG V=1.23\n# comment\nFROM --platform=$BUILDPLATFORM golang:${V} AS build\nRUN CGO_ENABLED=0 \\\n  # inner comment\n  go build -trimpath -o /x .\nFROM scratch\nCOPY --from=build /x /x\n"
	_, st := parseDockerfile(src)
	if len(st) != 2 || st[0].image != "golang:1.23" || st[0].name != "build" || st[1].image != "scratch" {
		t.Fatalf("парсер: %+v", st)
	}
	runs := st[0].find("RUN")
	if len(runs) != 1 || !strings.Contains(runs[0].args, "go build -trimpath") || runs[0].line != 4 {
		t.Fatalf("продолжение строк: %+v", runs)
	}
}
