//go:build !solution

// Package anagrams — задача к уроку «Работа с картами (map)».
package anagrams

// GroupAnagrams группирует слова-анаграммы (без учёта регистра, по рунам).
// Порядок детерминирован, несмотря на случайный порядок обхода map:
//   - группы идут в порядке первого появления их слова во входе;
//   - слова внутри группы — в порядке входа (дубликаты сохраняются).
//
// Пустой вход → пустой результат (len 0).
func GroupAnagrams(words []string) [][]string {
	// TODO: реализуйте
	panic("TODO")
}

// Invert «переворачивает» map: значение → отсортированный список ключей.
// Invert(nil) — пустая (не nil!) map, в которую можно писать.
func Invert(m map[string]int) map[int][]string {
	// TODO: реализуйте
	panic("TODO")
}

// TwoSum ищет индексы i < j, такие что nums[i]+nums[j] == target, за O(n).
// Если пар несколько — вернуть пару с минимальным j, а при равном j — с минимальным i.
// ok == false, если пары нет.
func TwoSum(nums []int, target int) (i, j int, ok bool) {
	// TODO: реализуйте
	panic("TODO")
}
