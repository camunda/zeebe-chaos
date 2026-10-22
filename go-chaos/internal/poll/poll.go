package poll

import "time"

// Poller bounds a polling loop by a total timeout, waiting interval between iterations.
type Poller struct {
	deadline time.Time
	interval time.Duration
	timedOut bool
}

func New(timeout time.Duration, interval time.Duration) *Poller {
	deadline := time.Now().Add(timeout)
	return &Poller{
		deadline: deadline,
		interval: interval,
	}
}

// TimedOut returns true when a loop over AttemptsNoSleep or Attempts stopped because another
// interval would pass the deadline. It stays false when the loop ended by break or return, and
// its value does not change with the time elapsed since the loop exited.
func (p *Poller) TimedOut() bool {
	return p.timedOut
}

// AttemptsNoSleep yields at least once and stops when another interval would pass the deadline.
func (p *Poller) AttemptsNoSleep() func(yield func(i int) bool) {
	return func(yield func(i int) bool) {
		p.timedOut = false
		for i := 0; ; i++ {
			if !yield(i) {
				return
			}

			if !time.Now().Add(p.interval).Before(p.deadline) {
				p.timedOut = true
				return
			}
		}
	}
}

// Attempts is AttemptsNoSleep with a sleep of interval before every iteration but the first.
func (p *Poller) Attempts() func(yield func(i int) bool) {
	return func(yield func(i int) bool) {
		for i := range p.AttemptsNoSleep() {
			if i > 0 {
				p.Wait()
			}
			if !yield(i) {
				return
			}
		}
	}
}

// Wait blocks for one interval.
func (p *Poller) Wait() {
	time.Sleep(p.interval)
}

// Remaining returns the time left until the deadline, negative once timed out.
func (p *Poller) Remaining() time.Duration {
	return time.Until(p.deadline)
}
