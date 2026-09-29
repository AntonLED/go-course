//go:build !solution

package slugtests

import "testing"

// CheckSlugify проверяет, что slugify ведёт себя по спецификации Slugify (см. slugify.go).
// Сообщайте о расхождениях через t.Errorf / t.Fatalf.
//
// Проверка будет запущена:
//   - на правильной реализации — ни одной ошибки быть не должно;
//   - на наборе «мутантов» (реализаций с тонкими багами) — каждый должен быть пойман.
//
// Подсказка: табличный тест []struct{ in, want string } и цикл по нему.
// Думайте о граничных случаях каждого из 5 правил.
func CheckSlugify(t testing.TB, slugify func(string) string) {
	t.Helper()
	// TODO: ваши тест-кейсы
}
