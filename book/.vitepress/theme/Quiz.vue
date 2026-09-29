<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from "vue";
import { state, load, loadQuiz, recordAnswer, resetQuiz, type Question, type QuizModule } from "./store";
import { hl, inline, esc } from "./hl";

const props = defineProps<{ module?: string; lesson?: string; exam?: boolean }>();

const mods = ref<QuizModule[]>([]);
const ready = ref(false);
const lessonsOn = ref<Set<string>>(new Set());
const shuffleOn = ref(true);

// прохождение
const phase = ref<"intro" | "run" | "done">("intro");
const list = ref<Question[]>([]);
const idx = ref(0);
const title = ref("");
const order = ref<number[]>([]);
const chosen = ref<number[]>([]);
const typed = ref("");
const checked = ref(false);
const lastOk = ref(false);
const results = ref<{ q: Question; ok: boolean; given?: string }[]>([]);
const root = ref<HTMLElement>();
const inputEl = ref<HTMLInputElement>();

const all = computed(() => mods.value.flatMap((m) => m.questions));
const allLessons = computed(() => [...new Set(all.value.map((q) => q.lesson))]);
const pool = computed(() => {
  if (props.exam) return all.value;
  if (props.lesson) return all.value.filter((q) => q.lesson === props.lesson);
  return all.value.filter((q) => lessonsOn.value.has(q.lesson));
});
const stat = (qs: Question[]) => {
  let ok = 0, bad = 0;
  for (const q of qs) { const r = state.quiz[q.id]; if (r) r.ok ? ok++ : bad++; }
  return { ok, bad, total: qs.length };
};
const poolStat = computed(() => stat(pool.value));
const unsolved = computed(() => pool.value.filter((q) => !state.quiz[q.id]?.ok));
const mistakes = computed(() => all.value.filter((q) => state.quiz[q.id] && !state.quiz[q.id].ok));
const cur = computed(() => list.value[idx.value]);
const score = computed(() => results.value.filter((r) => r.ok).length);
const wrong = computed(() => results.value.filter((r) => !r.ok));

const shuffle = <T,>(a: T[]) => { a = a.slice(); for (let i = a.length - 1; i > 0; i--) { const j = (Math.random() * (i + 1)) | 0; [a[i], a[j]] = [a[j], a[i]]; } return a; };
const norm = (s: string) => String(s).trim().replace(/\s+/g, " ").toLowerCase();
const pct = (n: number, t: number) => (t ? (100 * n) / t : 0) + "%";

onMounted(async () => {
  load();
  mods.value = await loadQuiz(props.exam ? undefined : props.module);
  lessonsOn.value = new Set(allLessons.value);
  ready.value = true;
  window.addEventListener("keydown", onKey);
});
onBeforeUnmount(() => window.removeEventListener("keydown", onKey));

function toggleLesson(l: string) {
  const s = new Set(lessonsOn.value);
  s.has(l) ? s.delete(l) : s.add(l);
  lessonsOn.value = s;
}

function start(qs: Question[], t: string, limit?: number) {
  let l = shuffleOn.value || props.exam ? shuffle(qs) : qs.slice();
  if (limit) l = l.slice(0, limit);
  if (!l.length) return;
  list.value = l; idx.value = 0; results.value = []; title.value = t;
  phase.value = "run";
  prep();
  nextTick(() => root.value?.scrollIntoView({ block: "start", behavior: "smooth" }));
}
function prep() {
  const q = cur.value;
  chosen.value = []; typed.value = ""; checked.value = false;
  order.value = q.options ? shuffle(q.options.map((_, i) => i)) : [];
  nextTick(() => inputEl.value?.focus());
}
function pick(i: number) {
  if (checked.value) return;
  const q = cur.value;
  if (q.type === "multi") chosen.value = chosen.value.includes(i) ? chosen.value.filter((x) => x !== i) : [...chosen.value, i];
  else chosen.value = [i];
}
const canCheck = computed(() => (cur.value?.type === "input" ? typed.value.trim() !== "" : chosen.value.length > 0));
function check() {
  if (!canCheck.value || checked.value) return;
  const q = cur.value;
  let ok: boolean;
  if (q.type === "input") ok = [q.answer as string, ...(q.accept || [])].some((a) => norm(a) === norm(typed.value));
  else {
    const a = (q.answer as number[]).slice().sort(), c = chosen.value.slice().sort();
    ok = a.length === c.length && a.every((v, k) => v === c[k]);
  }
  checked.value = true; lastOk.value = ok;
  recordAnswer(q.id, ok);
  results.value.push({ q, ok, given: q.type === "input" ? typed.value : undefined });
}
function next() {
  if (idx.value + 1 < list.value.length) { idx.value++; prep(); }
  else { phase.value = "done"; nextTick(() => root.value?.scrollIntoView({ block: "start", behavior: "smooth" })); }
}
function quit() { results.value.length ? (phase.value = "done") : (phase.value = "intro"); }
function onKey(e: KeyboardEvent) {
  if (phase.value !== "run" || e.metaKey || e.ctrlKey || e.altKey) return;
  const tag = (e.target as HTMLElement)?.tagName;
  if (e.key === "Enter") { e.preventDefault(); checked.value ? next() : check(); return; }
  if (tag === "INPUT" || tag === "TEXTAREA") return;
  if (/^[1-9]$/.test(e.key) && order.value.length) {
    const i = order.value[+e.key - 1];
    if (i !== undefined) pick(i);
  }
}
function optClass(i: number) {
  const q = cur.value, isC = chosen.value.includes(i);
  if (!checked.value) return isC ? "sel" : "";
  const isR = (q.answer as number[]).includes(i);
  return isR ? (isC ? "right picked" : "right") : isC ? "wrong" : "dim";
}
const answerText = (q: Question) =>
  q.type === "input" ? `<code>${esc(q.answer as string)}</code>` : (q.answer as number[]).map((k) => inline(q.options![k])).join("<br>");
let armed = false;
const resetLabel = ref("Сбросить прогресс");
function reset() {
  if (!armed) { armed = true; resetLabel.value = "Точно? Нажми ещё раз"; setTimeout(() => { armed = false; resetLabel.value = "Сбросить прогресс"; }, 3000); return; }
  armed = false; resetLabel.value = "Сбросить прогресс"; resetQuiz();
}
</script>

<template>
  <div ref="root" class="gq" :class="{ compact: !!lesson }">
    <div v-if="!ready" class="gq-card gq-muted">Загружаю вопросы…</div>

    <!-- ВСТУПЛЕНИЕ -->
    <div v-else-if="phase === 'intro'" class="gq-card">
      <template v-if="exam">
        <div class="gq-head">
          <div>
            <div class="gq-big-num">{{ stat(all).ok }} <span class="gq-muted">/ {{ all.length }}</span></div>
            <div class="gq-muted">верных ответов по всему курсу · ошибок: {{ stat(all).bad }}</div>
          </div>
          <button class="gq-link" @click="reset">{{ resetLabel }}</button>
        </div>
        <div class="gq-bar"><i class="g" :style="{ width: pct(stat(all).ok, all.length) }"></i><i class="r" :style="{ width: pct(stat(all).bad, all.length) }"></i></div>
        <div class="gq-mods">
          <div v-for="m in mods" :key="m.module" class="gq-mod">
            <span class="gq-muted">{{ m.module }}</span> {{ m.title }}
            <span class="gq-muted gq-right">{{ stat(m.questions).ok }}/{{ m.questions.length }}</span>
            <div class="gq-bar thin"><i class="g" :style="{ width: pct(stat(m.questions).ok, m.questions.length) }"></i><i class="r" :style="{ width: pct(stat(m.questions).bad, m.questions.length) }"></i></div>
          </div>
        </div>
        <div class="gq-row">
          <button class="gq-btn primary" @click="start(all, 'Экзамен', 40)">Экзамен · 40 случайных</button>
          <button class="gq-btn" :disabled="!mistakes.length" @click="start(mistakes, 'Работа над ошибками')">Работа над ошибками · {{ mistakes.length }}</button>
          <button class="gq-btn" :disabled="!all.filter(q => !state.quiz[q.id]).length" @click="start(all.filter(q => !state.quiz[q.id]), 'Ещё не отвеченные', 40)">Новые вопросы</button>
        </div>
      </template>

      <template v-else-if="lesson">
        <div class="gq-head">
          <div><b>{{ pool.length }} вопросов</b> по уроку <span class="gq-muted">· верно {{ poolStat.ok }}<template v-if="poolStat.bad">, ошибок {{ poolStat.bad }}</template></span></div>
        </div>
        <div class="gq-bar"><i class="g" :style="{ width: pct(poolStat.ok, poolStat.total) }"></i><i class="r" :style="{ width: pct(poolStat.bad, poolStat.total) }"></i></div>
        <div class="gq-row">
          <button class="gq-btn primary" @click="start(pool, 'Урок: ' + lesson)">Пройти тест</button>
          <button v-if="unsolved.length && unsolved.length < pool.length" class="gq-btn" @click="start(unsolved, 'Урок: ' + lesson)">Только нерешённые · {{ unsolved.length }}</button>
        </div>
      </template>

      <template v-else>
        <div class="gq-head">
          <div><b>{{ all.length }} вопросов</b> <span class="gq-muted">· верно {{ stat(all).ok }}<template v-if="stat(all).bad">, ошибок {{ stat(all).bad }}</template></span></div>
        </div>
        <div class="gq-bar"><i class="g" :style="{ width: pct(stat(all).ok, all.length) }"></i><i class="r" :style="{ width: pct(stat(all).bad, all.length) }"></i></div>
        <div class="gq-muted gq-small">Уроки (клик — включить/выключить):</div>
        <div class="gq-chips">
          <span v-for="l in allLessons" :key="l" class="gq-chip" :class="{ on: lessonsOn.has(l) }" @click="toggleLesson(l)">
            {{ l }} <span class="gq-muted">{{ stat(all.filter(q => q.lesson === l)).ok }}/{{ all.filter(q => q.lesson === l).length }}</span>
          </span>
        </div>
        <div class="gq-row">
          <button class="gq-btn primary" :disabled="!pool.length" @click="start(pool, 'Модуль ' + module)">Начать · {{ pool.length }}</button>
          <button class="gq-btn" :disabled="!unsolved.length" @click="start(unsolved, 'Модуль ' + module + ': нерешённые')">Только нерешённые · {{ unsolved.length }}</button>
          <label class="gq-check"><input type="checkbox" v-model="shuffleOn"> перемешать</label>
        </div>
      </template>
    </div>

    <!-- ВОПРОС -->
    <div v-else-if="phase === 'run' && cur" class="gq-card">
      <div class="gq-meta">
        <span>{{ title }} · {{ idx + 1 }} / {{ list.length }}</span>
        <span v-if="!lesson">{{ cur._mod }} · {{ cur.lesson }}</span>
      </div>
      <div class="gq-bar thin"><i class="a" :style="{ width: pct(idx + (checked ? 1 : 0), list.length) }"></i></div>
      <div class="gq-q" v-html="inline(cur.q)"></div>
      <pre v-if="cur.code" class="gq-code"><code v-html="hl(cur.code)"></code></pre>
      <div v-if="cur.type === 'multi'" class="gq-muted gq-small">Несколько верных ответов</div>

      <div v-if="cur.type !== 'input'" class="gq-opts">
        <button v-for="(i, k) in order" :key="i" class="gq-opt" :class="optClass(i)" @click="pick(i)" :disabled="checked">
          <span class="gq-k">{{ k + 1 }}</span>
          <span class="gq-box" :class="cur.type"></span>
          <span class="gq-txt" v-html="inline(cur.options![i])"></span>
        </button>
      </div>
      <input v-else ref="inputEl" v-model="typed" class="gq-input" :class="checked ? (lastOk ? 'right' : 'wrong') : ''" :disabled="checked" placeholder="Ответ…" autocomplete="off" spellcheck="false" />

      <div v-if="checked" class="gq-fb" :class="lastOk ? 'ok' : 'bad'">
        <b>{{ lastOk ? "Верно" : "Неверно" }}</b>
        <div v-if="!lastOk && cur.type === 'input'">Правильный ответ: <span v-html="answerText(cur)"></span></div>
        <div v-if="cur.explain" v-html="inline(cur.explain)"></div>
      </div>

      <div class="gq-actions">
        <button class="gq-link" @click="quit">← Выйти</button>
        <div class="gq-row nomargin">
          <span class="gq-muted gq-small gq-hide-sm">Enter — {{ checked ? "дальше" : "проверить" }}<template v-if="order.length && !checked">, 1–{{ order.length }} — выбор</template></span>
          <button v-if="!checked" class="gq-btn primary" :disabled="!canCheck" @click="check">Проверить</button>
          <button v-else class="gq-btn primary" @click="next">{{ idx + 1 < list.length ? "Дальше →" : "Результаты" }}</button>
        </div>
      </div>
    </div>

    <!-- ИТОГ -->
    <div v-else-if="phase === 'done'" class="gq-card">
      <div class="gq-center">
        <div class="gq-muted">{{ title }}</div>
        <div class="gq-score">{{ score }} / {{ results.length }}</div>
        <div class="gq-muted">{{ results.length ? Math.round((100 * score) / results.length) : 0 }}% верных</div>
        <div class="gq-row center">
          <button v-if="wrong.length" class="gq-btn primary" @click="start(wrong.map(r => r.q), title + ' · ошибки')">Повторить ошибки · {{ wrong.length }}</button>
          <button class="gq-btn" @click="phase = 'intro'">Готово</button>
        </div>
      </div>
      <div v-if="wrong.length" class="gq-review">
        <div class="gq-review-title">Разбор ошибок</div>
        <div v-for="r in wrong" :key="r.q.id" class="gq-item">
          <div class="gq-muted gq-small">{{ r.q._mod }} · {{ r.q.lesson }}</div>
          <div class="gq-q" v-html="inline(r.q.q)"></div>
          <pre v-if="r.q.code" class="gq-code"><code v-html="hl(r.q.code)"></code></pre>
          <div v-if="r.given" class="gq-small">Твой ответ: <code>{{ r.given }}</code></div>
          <div class="gq-small">Правильно:<br><span v-html="answerText(r.q)"></span></div>
          <div v-if="r.q.explain" class="gq-muted gq-small gq-explain" v-html="inline(r.q.explain)"></div>
        </div>
      </div>
    </div>
  </div>
</template>
