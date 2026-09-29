import { reactive } from "vue";

// Прогресс хранится только в браузере (localStorage). Всё обёрнуто в try/catch:
// в приватном режиме хранилище может быть недоступно — тогда просто не запоминаем.
const QKEY = "gocourse-quiz-v1";
const TKEY = "gocourse-tasks-v1";

export const state = reactive<{
  loaded: boolean;
  quiz: Record<string, { ok: boolean; n: number }>;
  tasks: Record<string, boolean>;
}>({ loaded: false, quiz: {}, tasks: {} });

export function load() {
  if (state.loaded || typeof window === "undefined") return;
  try { state.quiz = JSON.parse(localStorage.getItem(QKEY) || "{}") || {}; } catch {}
  try { state.tasks = JSON.parse(localStorage.getItem(TKEY) || "{}") || {}; } catch {}
  state.loaded = true;
  window.addEventListener("storage", (e) => {
    if (e.key === QKEY || e.key === TKEY) { state.loaded = false; load(); }
  });
}

export function recordAnswer(id: string, ok: boolean) {
  const prev = state.quiz[id];
  state.quiz[id] = { ok, n: (prev?.n || 0) + 1 };
  try { localStorage.setItem(QKEY, JSON.stringify(state.quiz)); } catch {}
}

export function setTask(id: string, done: boolean) {
  if (done) state.tasks[id] = true; else delete state.tasks[id];
  try { localStorage.setItem(TKEY, JSON.stringify(state.tasks)); } catch {}
}

export function resetQuiz() {
  state.quiz = {};
  try { localStorage.removeItem(QKEY); } catch {}
}

export type Question = {
  id: string; lesson: string; type: "single" | "multi" | "input";
  q: string; code?: string; options?: string[]; answer: number[] | string; accept?: string[]; explain?: string;
  _mod?: string; _modTitle?: string;
};
export type QuizModule = { module: string; title: string; questions: Question[] };

const loaders = import.meta.glob<{ default: QuizModule }>("../generated/quiz/*.json");

export async function loadQuiz(num?: string): Promise<QuizModule[]> {
  const keys = Object.keys(loaders).sort().filter((k) => !num || k.endsWith(`/${num}.json`));
  const mods = await Promise.all(keys.map((k) => loaders[k]().then((m) => m.default)));
  for (const m of mods) for (const q of m.questions) { q._mod = m.module; q._modTitle = m.title; }
  return mods;
}
