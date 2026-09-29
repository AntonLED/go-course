# Удобные цели. Номер модуля: make check M=3
M ?=

check:        ## прогресс по задачам (все или модуль M)
	go run ./cmd/check $(M)

race:         ## то же с детектором гонок
	go run ./cmd/check -race -v $(M)

solutions:    ## убедиться, что эталоны зелёные
	go run ./cmd/check -solution $(M)

quiz:         ## веб-тесты на http://localhost:8080
	go run ./cmd/quiz

quiz-build:   ## собрать quiz/data.js для открытия index.html без сервера
	go run ./cmd/quiz -build

book:         ## книга на VitePress: http://localhost:5173
	cd book && ([ -d node_modules ] || npm install) && npm run dev

.PHONY: check race solutions quiz quiz-build book
