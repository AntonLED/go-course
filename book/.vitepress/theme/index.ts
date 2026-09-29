import DefaultTheme from "vitepress/theme";
import type { Theme } from "vitepress";
import Quiz from "./Quiz.vue";
import CourseMap from "./CourseMap.vue";
import TaskBadge from "./TaskBadge.vue";
import "katex/dist/katex.min.css";
import "./custom.css";

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component("Quiz", Quiz);
    app.component("CourseMap", CourseMap);
    app.component("TaskBadge", TaskBadge);
  },
} satisfies Theme;
