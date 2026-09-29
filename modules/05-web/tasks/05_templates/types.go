package pages

// Item — элемент каталога.
type Item struct {
	Title      string
	URL        string
	Tags       []string
	PriceCents int64
}

// Page — данные страницы.
type Page struct {
	Title string
	User  string // пусто — аноним
	Items []Item
}
