<script setup lang="ts">
import { computed, onMounted } from "vue";
import { state, load, setTask } from "./store";

const props = defineProps<{ id: string }>();
onMounted(load);
const done = computed(() => !!state.tasks[props.id]);
const isProject = computed(() => props.id.includes("_project_"));
</script>

<template>
  <div class="tb">
    <span v-if="isProject" class="tb-tag">★ Финальное задание модуля</span>
    <span v-else class="tb-tag muted">Задача</span>
    <button class="tb-btn" :class="{ done }" @click="setTask(id, !done)">
      {{ done ? "✓ Решена" : "Отметить решённой" }}
    </button>
  </div>
</template>
