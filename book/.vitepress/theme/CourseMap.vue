<script setup lang="ts">
import { onMounted, ref } from "vue";
import { withBase } from "vitepress";
import modules from "../generated/modules.json";
import sidebar from "../generated/sidebar.json";
import { state, load, loadQuiz, type QuizModule } from "./store";

const quiz = ref<Record<string, string[]>>({});
onMounted(async () => {
  load();
  const mods: QuizModule[] = await loadQuiz();
  quiz.value = Object.fromEntries(mods.map((m) => [m.module, m.questions.map((q) => q.id)]));
});

// id задач из сайдбара: /modules/03-concurrency/tasks/08_errgroup/ → 03-concurrency/08_errgroup
const taskIds: Record<string, string[]> = {};
for (const g of sidebar as any[]) {
  for (const it of g.items || []) for (const t of it.items || []) {
    const m = t.link.match(/^\/modules\/([^/]+)\/tasks\/([^/]+)\//);
    if (m) (taskIds[m[1]] ||= []).push(`${m[1]}/${m[2]}`);
  }
}
const solved = (dir: string) => (taskIds[dir] || []).filter((id) => state.tasks[id]).length;
const qok = (num: string) => (quiz.value[num] || []).filter((id) => state.quiz[id]?.ok).length;
const pct = (a: number, b: number) => (b ? (100 * a) / b : 0) + "%";
</script>

<template>
  <div class="cm">
    <a v-for="m in modules" :key="m.num" class="cm-card" :href="withBase(`/modules/${m.dir}/`)">
      <div class="cm-num">МОДУЛЬ {{ m.num }}</div>
      <div class="cm-title">{{ m.title }}</div>
      <div class="cm-meta">{{ m.lessons }} уроков · {{ m.tasks }} задач · {{ m.questions }} вопросов</div>
      <div class="cm-line"><span>Задачи</span><span>{{ solved(m.dir) }}/{{ m.tasks }}</span></div>
      <div class="gq-bar thin"><i class="g" :style="{ width: pct(solved(m.dir), m.tasks) }"></i></div>
      <div class="cm-line"><span>Тест</span><span>{{ qok(m.num) }}/{{ m.questions }}</span></div>
      <div class="gq-bar thin"><i class="a" :style="{ width: pct(qok(m.num), m.questions) }"></i></div>
    </a>
  </div>
  <p class="cm-note">
    Прогресс по тестам считается автоматически. Задачу можно отметить кнопкой «Решена» на её странице,
    когда <code>go test</code> позеленеет, а точную сводку всегда покажет <code>go run ./cmd/check</code>.
    Отметки хранятся в этом браузере.
  </p>
</template>
