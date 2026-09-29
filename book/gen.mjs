// Генератор книги: собирает markdown из ../modules в структуру VitePress.
//
//   node gen.mjs          — сгенерировать страницы
//   node gen.mjs --watch  — сгенерировать и пересобирать при изменении ../modules
//
// Источник правды — файлы в modules/. Всё, что пишет этот скрипт, можно удалить:
// book/modules, book/index.md, book/exam.md, book/.vitepress/generated/.
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import katex from "katex";

const BOOK = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(BOOK, "..");
const MODULES = path.join(ROOT, "modules");
const OUT = path.join(BOOK, "modules");
const GEN = path.join(BOOK, ".vitepress", "generated");

// Названия уроков из силлабуса — в порядке файлов lessons/NN-*.md.
const SYLLABUS = {
  "01": ["Осознанное знакомство с Go", "Основы синтаксиса", "Работа с массивами и срезами", "Работа со строками", "Работа с картами (map): создание, доступ, операции", "Указатели, структуры, методы"],
  "02": ["Интерфейсы", "Обработка ошибок в Go", "Управление пакетами и модулями", "Дженерики", "Итераторы"],
  "03": ["Введение в параллельное программирование. Модель PMG", "Горутины", "Синхронизация данных", "Concurrency в Go: каналы и паттерны", "Контекст (Context)", "Concurrency в Go: errgroup, singleflight"],
  "04": ["Работа с потоками ввода/вывода", "Работа с аргументами командной строки", "Работа с файлами", "Работа с JSON, YAML", "Работа с SQL базами данных"],
  "05": ["Основы HTTP и запуск сервера в Go", "Роутинг и middleware", "Работа с запросами и ответами", "Шаблоны и статические файлы", "HTTP-клиент в Go", "Популярные фреймворки для HTTP"],
  "06": ["Тестирование в Go", "Мокирование и тестирование API", "Бенчмарки", "Профилирование"],
  "07": ["Введение в микросервисы", "JSON-RPC", "gRPC + protobuf"],
  "08": ["TLS, Сертификаты, Цепочки сертификатов", "Безопасность в HTTP (HTTPS)", "Безопасность в gRPC", "Аутентификация и авторизация (JWT)", "Аутентификация и авторизация (OAuth 2.0)"],
  "09": ["Рефлексия", "Внедрение зависимостей (DI)", "Управление конфигурациями и средами", "Управление памятью и аллокациями", "unsafe", "Логгирование, трейсинг, метрики", "Сборка Docker-контейнера"],
};

const ls = (d) => (fs.existsSync(d) ? fs.readdirSync(d).sort() : []);
const read = (f) => fs.readFileSync(f, "utf8");
function write(f, s) {
  fs.mkdirSync(path.dirname(f), { recursive: true });
  if (fs.existsSync(f) && read(f) === s) return; // не дёргать HMR зря
  fs.writeFileSync(f, s);
}
const firstH1 = (md) => (md.match(/^#\s+(.+)$/m) || [, ""])[1].trim();
const plain = (s) => s.replace(/`/g, "").replace(/\[([^\]]*)\]\([^)]*\)/g, "$1").replace(/\*\*/g, "").trim();

// ---------- переписывание ссылок ----------
// Работает вне ``` блоков и вне `inline code`.
function rewriteLinks(md, srcFile) {
  const dir = path.dirname(srcFile);
  const fix = (text, target) => {
    if (/^(https?:|mailto:|#)/.test(target)) return null;
    const [p, hash = ""] = target.split("#");
    const abs = path.resolve(dir, p);
    const h = hash ? "#" + hash : "";
    if (!abs.startsWith(MODULES)) return text ? `\`${text}\`` : ""; // за пределами курса
    if (fs.existsSync(abs) && fs.statSync(abs).isDirectory()) {
      if (fs.existsSync(path.join(abs, "README.md"))) return `[${text}](${p.replace(/\/?$/, "/")}${h})`;
      return text;
    }
    if (/README\.md$/.test(p)) return `[${text}](${p.replace(/README\.md$/, "")}${h})`;
    if (/\.md$/.test(p)) return null; // ок как есть
    // .go, .json, Dockerfile и т.п. — не страницы: оставляем текст
    return /`/.test(text) ? text : `\`${text}\``;
  };
  const re = /(`[^`]*`)|\[((?:`[^`]*`|[^\]`])*)\]\(([^)\s]+)\)/g;
  let inFence = false;
  return md
    .split("\n")
    .map((line) => {
      if (/^\s*(```|~~~)/.test(line)) { inFence = !inFence; return line; }
      if (inFence) return line;
      return line.replace(re, (m, code, text, target) => {
        if (code) return m;
        const r = fix(text, target);
        return r === null ? m : r;
      });
    })
    .join("\n");
}


// ---------- схемы ----------
// Строка вида ![Подпись](img/name.svg) заменяется на встроенный <figure> с SVG,
// чтобы схема наследовала цвета темы (currentColor) и переключалась со светлой/тёмной.
// id внутри SVG префиксуются именем файла — несколько схем на странице не конфликтуют.

// Подпись к схеме: `код` и формулы $...$ (KaTeX) — markdown внутри <figure> не обрабатывается.
function capHTML(cap) {
  return cap
    .split(/(`[^`]+`)/)
    .map((p) => /^`[^`]+`$/.test(p)
      ? `<code>${p.slice(1, -1)}</code>`
      : p.replace(/\$([^$\n]+?)\$/g, (_, t) => katex.renderToString(t.replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&amp;/g, "&"), { output: "html", throwOnError: false })))
    .join("");
}

function inlineDiagrams(md, srcFile) {
  const dir = path.dirname(srcFile);
  let inFence = false;
  return md
    .split("\n")
    .map((line) => {
      if (/^\s*(```|~~~)/.test(line)) { inFence = !inFence; return line; }
      if (inFence) return line;
      const m = line.match(/^\s*!\[((?:`[^`]*`|[^\]`])*)\]\(([^)\s]+\.svg)\)\s*$/);
      if (!m) return line;
      const abs = path.resolve(dir, m[2]);
      if (!fs.existsSync(abs)) throw new Error(`${srcFile}: нет файла схемы ${m[2]}`);
      const pre = "d-" + path.basename(abs, ".svg").replace(/[^a-zA-Z0-9_-]/g, "") + "-";
      let svg = read(abs)
        .replace(/<\?xml[^>]*>/g, "")
        .replace(/<!--[\s\S]*?-->/g, "")
        .replace(/\bid="([^"]+)"/g, (_, id) => `id="${pre}${id}"`)
        .replace(/url\(#([^)]+)\)/g, (_, id) => `url(#${pre}${id})`)
        .replace(/href="#([^"]+)"/g, (_, id) => `href="#${pre}${id}"`)
        .split("\n").map((l) => l.trim()).filter(Boolean).join("\n");
      const cap = m[1].trim().replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
      return `<figure class="diagram" v-pre>\n${svg}\n${cap ? `<figcaption>${capHTML(cap)}</figcaption>\n` : ""}</figure>`;
    })
    .join("\n");
}

const langOf = (f) =>
  f.endsWith(".go") ? "go" : /Dockerfile/.test(f) ? "dockerfile" : f.endsWith(".json") ? "json" : f.endsWith(".yaml") || f.endsWith(".yml") ? "yaml" : f.endsWith(".md") ? "md" : "text";

function taskFiles(dir) {
  const files = ls(dir).filter((f) => fs.statSync(path.join(dir, f)).isFile() && f !== "README.md");
  const tests = files.filter((f) => f.endsWith("_test.go"));
  const sol = files.filter((f) => /_solution\.go$|\.solution(\.|$)/.test(f));
  const stub = files.filter((f) => !tests.includes(f) && !sol.includes(f));
  return { stub, tests, sol };
}

function snippet(absFile, title) {
  const rel = path.relative(BOOK, absFile).split(path.sep).join("/");
  return `::: details ${title}\n<<< @/${rel}{${langOf(absFile)}}\n:::\n`;
}

// ---------- генерация ----------
function generate() {
  const t0 = Date.now();
  const sidebar = [];
  const modulesMeta = [];
  const quizDir = path.join(GEN, "quiz");
  fs.mkdirSync(quizDir, { recursive: true });

  for (const mdir of ls(MODULES).filter((d) => /^\d\d-/.test(d))) {
    const num = mdir.slice(0, 2);
    const src = path.join(MODULES, mdir);
    const dst = path.join(OUT, mdir);
    const quiz = JSON.parse(read(path.join(src, "quiz.json")));
    fs.copyFileSync(path.join(src, "quiz.json"), path.join(quizDir, `${num}.json`));
    const lessonNames = SYLLABUS[num] || [];
    const items = [];

    // обзор модуля
    const readme = path.join(src, "README.md");
    write(path.join(dst, "index.md"), rewriteLinks(inlineDiagrams(read(readme), readme), readme) +
      `\n\n## Тест по модулю\n\n<Quiz module="${num}" />\n`);
    items.push({ text: "Обзор модуля", link: `/modules/${mdir}/` });

    // уроки
    const lessons = ls(path.join(src, "lessons")).filter((f) => f.endsWith(".md"));
    lessons.forEach((f, i) => {
      const file = path.join(src, "lessons", f);
      const name = lessonNames[i] || plain(firstH1(read(file)));
      const qn = quiz.questions.filter((q) => q.lesson === name).length;
      let body = rewriteLinks(inlineDiagrams(read(file), file), file);
      if (qn) body += `\n\n## Проверь себя\n\n<Quiz module="${num}" lesson="${name.replace(/"/g, "&quot;")}" />\n`;
      write(path.join(dst, "lessons", f), `---\ntitle: "${name.replace(/"/g, '\\"')}"\n---\n\n` + body);
      items.push({ text: `${i + 1}. ${name}`, link: `/modules/${mdir}/lessons/${f.replace(/\.md$/, "")}` });
    });

    // задачи
    const tasks = [];
    for (const t of ls(path.join(src, "tasks"))) {
      const tdir = path.join(src, "tasks", t);
      const tr = path.join(tdir, "README.md");
      if (!fs.existsSync(tr)) continue;
      const md = read(tr);
      const title = plain(firstH1(md)).replace(/^\d+\.\s*/, "");
      const isProject = /_project_/.test(t);
      const { stub, tests, sol } = taskFiles(tdir);
      const pkgPath = `./modules/${mdir}/tasks/${t}/`;
      let extra = /go test/.test(md) ? `\n\n---\n\n` : `\n\n---\n\n## Проверка\n\n\`\`\`bash\n# из корня курса\ngo test ${pkgPath}\ngo test -race ${pkgPath}\n\n# эталонное решение\ngo test -tags solution ${pkgPath}\n\`\`\`\n\n`;
      extra += `## Файлы задачи\n\n`;
      extra += `Решение пишется в файлах заготовки в папке \`modules/${mdir}/tasks/${t}/\`.\n\n`;
      for (const f of stub) extra += snippet(path.join(tdir, f), `Заготовка · ${f}`);
      for (const f of tests) extra += snippet(path.join(tdir, f), `Тесты · ${f}`);
      if (sol.length) {
        extra += `\n### Эталонное решение\n\n::: danger Спойлер\nЛучше открывать, когда ваши тесты уже зелёные или если вы застряли всерьёз.\n:::\n\n`;
        for (const f of sol) extra += snippet(path.join(tdir, f), `Решение · ${f}`);
      }
      write(path.join(dst, "tasks", t, "index.md"),
        `---\ntitle: "${title.replace(/"/g, '\\"')}"\n---\n\n<TaskBadge id="${mdir}/${t}" />\n\n` + rewriteLinks(inlineDiagrams(md, tr), tr) + extra);
      tasks.push({ text: (isProject ? "★ " : "") + `${t.slice(0, 2)}. ${title}`, link: `/modules/${mdir}/tasks/${t}/` });
    }
    items.push({ text: `Задачи (${tasks.length})`, collapsed: true, items: tasks });
    items.push({ text: "Тест модуля", link: `/modules/${mdir}/quiz` });
    write(path.join(dst, "quiz.md"), `---\ntitle: "Тест: ${quiz.title}"\naside: false\n---\n\n# Тест · ${quiz.title}\n\n<Quiz module="${num}" />\n`);

    sidebar.push({ text: `${num} · ${quiz.title}`, collapsed: true, items });
    modulesMeta.push({ num, dir: mdir, title: quiz.title, lessons: lessons.length, tasks: tasks.length, questions: quiz.questions.length });
  }

  sidebar.unshift({ text: "Курс", items: [{ text: "Главная", link: "/" }, { text: "Экзамен и ошибки", link: "/exam" }] });
  write(path.join(GEN, "sidebar.json"), JSON.stringify(sidebar, null, 2));
  write(path.join(GEN, "modules.json"), JSON.stringify(modulesMeta, null, 2));

  // главная
  const total = modulesMeta.reduce((a, m) => ({ l: a.l + m.lessons, t: a.t + m.tasks, q: a.q + m.questions }), { l: 0, t: 0, q: 0 });
  write(path.join(BOOK, "index.md"), `---
layout: home
hero:
  name: "Go: подготовка"
  text: "Конспекты, задачи и тесты"
  tagline: "${modulesMeta.length} модулей · ${total.l} уроков · ${total.t} задач с автотестами · ${total.q} вопросов"
  actions:
    - theme: brand
      text: Начать с модуля 01
      link: /modules/${modulesMeta[0].dir}/
    - theme: alt
      text: Экзамен
      link: /exam
features:
  - title: Конспекты
    details: Не только как пользоваться, но и как это устроено внутри и где обычно ошибаются. В конце каждого урока есть короткий тест.
  - title: Задачи с тестами
    details: Пишете код в заготовке, пока go test не позеленеет. Эталонное решение спрятано под спойлером на странице задачи.
  - title: Только stdlib
    details: Ничего не нужно скачивать. Там, где в проекте взяли бы библиотеку, вы пишете её механизм сами.
---

<CourseMap />
`);
  write(path.join(BOOK, "exam.md"), `---\ntitle: Экзамен\naside: false\n---\n\n# Экзамен и работа над ошибками\n\n<Quiz exam />\n`);

  console.log(`[gen] ${modulesMeta.length} модулей, ${total.t} задач, ${total.q} вопросов · ${Date.now() - t0} мс`);
}

generate();

if (process.argv.includes("--watch")) {
  let timer;
  fs.watch(MODULES, { recursive: true }, (_ev, file) => {
    if (!file || /(^|\/)\./.test(file)) return;
    clearTimeout(timer);
    timer = setTimeout(() => { try { generate(); } catch (e) { console.error("[gen]", e.message); } }, 150);
  });
  console.log("[gen] слежу за modules/ …");
}
