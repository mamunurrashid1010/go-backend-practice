// A deliberately racy counter, so we have a test that trips -race
// but appears to "work" without it.
//
// Never write real code like this. If you need a shared counter, use
// sync/atomic or a sync.Mutex.
package race

// Counter — no synchronisation on n. Multi-goroutine increments race.
type Counter struct{ n int }

func (c *Counter) Inc() { c.n++ }
func (c *Counter) N() int { return c.n }
