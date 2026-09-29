package memopt

// EventOriginal — исходная («плохая») раскладка полей. Не меняйте её:
// тест сравнивает с ней набор полей вашего Event.
type EventOriginal struct {
	Active  bool
	ID      int64
	Kind    uint8
	Score   float64
	Flags   uint16
	Count   int32
	Deleted bool
	Name    string
}

// Record — запись для AppendRecord.
type Record struct {
	ID    int64
	Name  string
	Score float64
	OK    bool
}
