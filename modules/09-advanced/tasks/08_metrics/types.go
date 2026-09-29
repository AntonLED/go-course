package metrics

import "errors"

var (
	ErrInvalidName = errors.New("metrics: некорректное имя метрики или лейбла")
	ErrDuplicate   = errors.New("metrics: метрика с таким именем уже зарегистрирована")
	ErrBuckets     = errors.New("metrics: границы бакетов должны строго возрастать")
)

// DefBuckets — бакеты по умолчанию (секунды), как в prometheus/client_golang.
var DefBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

// ContentType — Content-Type текстового формата экспозиции Prometheus.
const ContentType = "text/plain; version=0.0.4; charset=utf-8"
