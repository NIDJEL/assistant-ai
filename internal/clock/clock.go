package clock

import "time"

type Clock interface{ Now() time.Time }
type System struct{ Location *time.Location }

func (s System) Now() time.Time { return time.Now().In(s.Location) }

type Func func() time.Time

func (f Func) Now() time.Time { return f() }
