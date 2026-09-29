import { defineConfig } from "vitepress";
import markdownItKatex from "@vscode/markdown-it-katex";
import fs from "node:fs";
import path from "node:path";

const gen = (f: string) => JSON.parse(fs.readFileSync(path.join(__dirname, "generated", f), "utf8"));

// На GitHub Pages сайт проекта живёт по адресу https://<user>.github.io/<repo>/,
// поэтому base задаётся при сборке через BASE_PATH (см. .github/workflows/pages.yml).
// Локально (npm run dev) base = "/".
const base = process.env.BASE_PATH || "/";

export default defineConfig({
  base,
  lang: "ru-RU",
  title: "Go: подготовка",
  description: "Курс подготовки по Go: конспекты, задачи с тестами, квизы",
  cleanUrls: true,
  lastUpdated: false,
  srcExclude: ["node_modules/**", "README.md"],
  head: [["link", { rel: "icon", href: "data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'><rect width='32' height='32' rx='7' fill='%2300ADD8'/><text x='16' y='22' font-size='15' font-family='monospace' font-weight='700' text-anchor='middle' fill='white'>go</text></svg>" }]],
  markdown: {
    theme: { light: "github-light", dark: "github-dark" },
    lineNumbers: false,
    config(md) {
      // Формулы: $...$ в строке и $$...$$ блоком (KaTeX, только HTML-вывод — без MathML).
      md.use((markdownItKatex as any).default ?? markdownItKatex, { output: "html", throwOnError: false });
      // Inline-код и обычный текст с {{ }} (Go-шаблоны) не должны интерпретироваться Vue.
      const code = md.renderer.rules.code_inline!;
      md.renderer.rules.code_inline = (tokens, idx, opts, env, self) =>
        code(tokens, idx, opts, env, self).replace(/^<code/, "<code v-pre");
      const text = md.renderer.rules.text!;
      md.renderer.rules.text = (tokens, idx, opts, env, self) => {
        const out = text(tokens, idx, opts, env, self);
        return out.includes("{{") ? `<span v-pre>${out}</span>` : out;
      };
    },
  },
  themeConfig: {
    logo: { src: "data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'><rect width='32' height='32' rx='7' fill='%2300ADD8'/><text x='16' y='22' font-size='15' font-family='monospace' font-weight='700' text-anchor='middle' fill='white'>go</text></svg>" },
    nav: [
      { text: "Модули", link: "/" },
      { text: "Экзамен", link: "/exam" },
    ],
    sidebar: gen("sidebar.json"),
    outline: { level: [2, 3], label: "На этой странице" },
    docFooter: { prev: "Назад", next: "Дальше" },
    darkModeSwitchLabel: "Тема",
    lightModeSwitchTitle: "Светлая тема",
    darkModeSwitchTitle: "Тёмная тема",
    sidebarMenuLabel: "Оглавление",
    returnToTopLabel: "Наверх",
    langMenuLabel: "Язык",
    notFound: { title: "Страница не найдена", quote: "Похоже, такой страницы нет.", linkText: "На главную" },
    search: {
      provider: "local",
      options: {
        translations: {
          button: { buttonText: "Поиск", buttonAriaLabel: "Поиск" },
          modal: {
            noResultsText: "Ничего не найдено",
            resetButtonTitle: "Сбросить",
            displayDetails: "Подробнее",
            footer: { selectText: "выбрать", navigateText: "навигация", closeText: "закрыть" },
          },
        },
      },
    },
  },
});
