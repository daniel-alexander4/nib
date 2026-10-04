package scaling

import (
	"fmt"
	"testing"
	"time"
)

// verdictTB records whether GrowsLinearly failed, and stops it there as a real Fatalf would.
type verdictTB struct {
	testing.TB
	failed string
}

type stopped struct{}

func (v *verdictTB) Helper()             {}
func (v *verdictTB) Logf(string, ...any) {}
func (v *verdictTB) Fatalf(f string, args ...any) {
	v.failed = fmt.Sprintf(f, args...)
	panic(stopped{})
}
func (v *verdictTB) Errorf(f string, args ...any) { v.Fatalf(f, args...) }

func verdict(small, large int, bound float64, cost func(n int) time.Duration) (failed string) {
	v := &verdictTB{}
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(stopped); !ok {
				panic(r)
			}
		}
		failed = v.failed
	}()
	GrowsLinearly(v, "probe", small, large, bound, cost)
	return v.failed
}

// The verdict's logic, on costs that are not clocks: a linear cost passes, a quadratic one fails, a
// spike covering ONE size in some rounds passes (the defect the separate-window shapes had), and a
// spike in every round is indistinguishable from a regression and fails — the door does not hide one.
func TestTheVerdictIsTheLeastInterleavedRatio(t *testing.T) {
	linear := func(n int) time.Duration { return time.Duration(n) * time.Microsecond }
	quadratic := func(n int) time.Duration { return time.Duration(n*n) * time.Nanosecond }
	measured := 0
	if f := verdict(1000, 4000, 8, func(n int) time.Duration { measured++; return linear(n) }); f != "" {
		t.Errorf("a linear cost failed: %s", f)
	}
	if measured != 3 { // a round within bound settles the verdict; a healthy run pays one round, not five
		t.Errorf("a linear cost was measured %d times, want 3 (one round: small, large, small)", measured)
	}
	smalls := 0
	bracketed := func(n int) time.Duration { // a quadratic whose every other small reading is inflated ×3
		if n == 1000 {
			if smalls++; smalls%2 == 0 {
				return 3 * quadratic(n)
			}
		}
		return quadratic(n)
	}
	if f := verdict(1000, 4000, 8, bracketed); f == "" {
		t.Error("a quadratic passed because a spike inflated one of the small readings around each large one")
	}
	if f := verdict(1000, 4000, 8, quadratic); f == "" {
		t.Error("a quadratic cost passed")
	}
	calls := 0
	spiky := func(n int) time.Duration { // every round but the last has the large size measured under a ×5 spike
		calls++
		d := linear(n)
		if n == 4000 && calls < 2*rounds {
			d *= 5
		}
		return d
	}
	if f := verdict(1000, 4000, 8, spiky); f != "" {
		t.Errorf("a spike over the large size in %d of %d rounds read as a regression: %s", rounds-1, rounds, f)
	}
	always := func(n int) time.Duration {
		if n == 4000 {
			return 5 * linear(n)
		}
		return linear(n)
	}
	if f := verdict(1000, 4000, 8, always); f == "" {
		t.Error("a ×20 ratio in every round passed")
	}
	if f := verdict(1000, 2000, 4, linear); f == "" {
		t.Error("a bound of ×4 for ×2 the size — the quadratic ratio itself — was accepted")
	}
	if f := verdict(1000, 4000, 4, linear); f == "" {
		t.Error("a bound of ×4 for ×4 the size — the linear ratio itself — was accepted")
	}
}

var sink []*[8]byte

// AllocsGrowLinearly counts: n heap allocations pass, n² fail, and a run that allocates nothing is refused
// rather than read as linear.
func TestAllocationsAreCountedNotTimed(t *testing.T) {
	run := func(n int) {
		sink = sink[:0]
		for i := 0; i < n; i++ {
			sink = append(sink[:0], new([8]byte))
		}
	}
	allocs := func(prep func(n int) func()) (failed string) {
		v := &verdictTB{}
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(stopped); !ok {
					panic(r)
				}
			}
			failed = v.failed
		}()
		AllocsGrowLinearly(v, "probe", 1000, 4000, 8, prep)
		return v.failed
	}
	if f := allocs(func(n int) func() { return func() { run(n) } }); f != "" {
		t.Errorf("n allocations failed: %s", f)
	}
	if f := allocs(func(n int) func() { return func() { run(n * n / 100) } }); f == "" {
		t.Error("n² allocations passed")
	}
	if f := allocs(func(n int) func() { return func() {} }); f == "" {
		t.Error("a run that allocates nothing passed")
	}
}

// WithinFactor is the same least ratio against a base: a cost spiking in some rounds passes, one over its
// bound in every round fails.
func TestABudgetedCostIsJudgedAgainstItsBase(t *testing.T) {
	within := func(bound float64, base, cost func() time.Duration) (failed string) {
		v := &verdictTB{}
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(stopped); !ok {
					panic(r)
				}
			}
			failed = v.failed
		}()
		WithinFactor(v, "probe", bound, base, cost)
		return v.failed
	}
	base := func() time.Duration { return time.Millisecond }
	calls := 0
	spiky := func() time.Duration { // ×3 the base, under a ×10 spike in every round but the last
		calls++
		if calls < rounds {
			return 30 * time.Millisecond
		}
		return 3 * time.Millisecond
	}
	if f := within(5, base, spiky); f != "" {
		t.Errorf("a spike in %d of %d rounds read as a breach: %s", rounds-1, rounds, f)
	}
	if f := within(5, base, func() time.Duration { return 6 * time.Millisecond }); f == "" {
		t.Error("×6 the base in every round passed a bound of ×5")
	}
}
