package pdfread

import (
	"runtime"
	"sync"
	"weak"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// optimizeSkips holds the contexts whose optimize pass was SKIPPED because it was not affordable
// (`optimize`'s nil return) — /pending 766 part 7.
//
// The skip is the right answer for the read: a reader that only wants the document still gets it. But the pass is
// also what merges a document's duplicate fonts and images, so a document written from such a context is larger
// than the same change would leave another one, and until this mark nothing downstream could tell: `optimize`
// returned nil either way and some twenty readers learned nothing. Kept against the context, as `rememberXFA`
// keeps its own reading, so the answer is free for a read that is already being made and nothing is stored for
// the nearly-every document whose pass ran.
var optimizeSkips = struct {
	sync.Mutex
	m map[weak.Pointer[model.Context]]struct{}
}{m: map[weak.Pointer[model.Context]]struct{}{}}

func rememberOptimizeSkipped(ctx *model.Context) {
	key := weak.Make(ctx)
	optimizeSkips.Lock()
	_, had := optimizeSkips.m[key]
	optimizeSkips.m[key] = struct{}{}
	optimizeSkips.Unlock()
	if had {
		return // one cleanup a context, however many times its pass is asked for
	}
	runtime.AddCleanup(ctx, func(k weak.Pointer[model.Context]) {
		optimizeSkips.Lock()
		delete(optimizeSkips.m, k)
		optimizeSkips.Unlock()
	}, key)
}

// OptimizeSkipped reports whether pdfcpu's optimize pass was asked for over ctx and skipped as unaffordable
// (`Unaffordable`). It says nothing of a context nobody asked to optimize, and it is not a refusal: the read
// succeeded. What it means for a WRITE is that the output keeps every duplicate the pass would have merged.
func OptimizeSkipped(ctx *model.Context) bool {
	optimizeSkips.Lock()
	defer optimizeSkips.Unlock()
	_, ok := optimizeSkips.m[weak.Make(ctx)]
	return ok
}
