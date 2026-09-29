package allocs

// Record — запись для AppendRecord.
type Record struct {
	ID     int64
	Name   string
	Score  float64
	Active bool
	Tags   []string
}
