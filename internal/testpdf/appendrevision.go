package testpdf

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	reStartxref = regexp.MustCompile(`startxref\s+(\d+)`)
	reSize      = regexp.MustCompile(`/Size\s+(\d+)`)
	reRoot      = regexp.MustCompile(`/Root\s+(\d+)\s+0\s+R`)
)

// AppendRevision writes objs (object number → body) as one incremental update of prev, with an uncompressed xref
// stream — a later revision built by hand, for tests of what a returned document can carry (PLAN-returned-document
// P02.S03). prev's last `startxref`, `/Size` and `/Root` are read from its bytes, so prev must end in a revision this
// function or a signer like pdfsign wrote — and in an XREF STREAM: the update's `/Prev` names prev's xref, and the
// digitorus reader follows `/Prev` only from stream to stream, so over a classic table the result reads as unreadable
// (the P02 phase-close review). It is `sign`'s `synthRevision` without the object-stream half; `sign`
// cannot be imported here (testpdf feeds its tests), so the server's tests build their fixtures with this.
func AppendRevision(prev []byte, objs map[int]string) ([]byte, error) {
	last := func(re *regexp.Regexp) (int, error) {
		all := re.FindAllSubmatch(prev, -1)
		if len(all) == 0 {
			return 0, fmt.Errorf("testpdf: no %s in the previous revision", re)
		}
		return strconv.Atoi(string(all[len(all)-1][1]))
	}
	prevX, err := last(reStartxref)
	if err != nil {
		return nil, err
	}
	size, err := last(reSize)
	if err != nil {
		return nil, err
	}
	root, err := last(reRoot)
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, errors.New("testpdf: an empty revision")
	}
	var b bytes.Buffer
	b.Write(prev)
	b.WriteString("\n")
	offsets := map[int]int{}
	var nums []int
	for n := range objs {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	next := size
	for _, n := range nums {
		if n >= next {
			next = n + 1
		}
		offsets[n] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, objs[n])
	}
	xnum := next
	offsets[xnum] = b.Len()
	nums = append(nums, xnum)
	var index strings.Builder
	var data bytes.Buffer
	for i := 0; i < len(nums); {
		j := i
		for j+1 < len(nums) && nums[j+1] == nums[j]+1 {
			j++
		}
		fmt.Fprintf(&index, "%d %d ", nums[i], j-i+1)
		for _, n := range nums[i : j+1] {
			o := offsets[n]
			data.Write([]byte{1, byte(o >> 24), byte(o >> 16), byte(o >> 8), byte(o), 0, 0})
		}
		i = j + 1
	}
	fmt.Fprintf(&b, "%d 0 obj\n<</Type/XRef/Size %d/W[1 4 2]/Index[%s]/Root %d 0 R/Prev %d/Length %d>>\nstream\n",
		xnum, xnum+1, strings.TrimSpace(index.String()), root, prevX, data.Len())
	b.Write(data.Bytes())
	fmt.Fprintf(&b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", offsets[xnum])
	return b.Bytes(), nil
}

// SignatureDictionary returns the object number and body (header excluded) of the first PPKLite signature dictionary
// in doc — what a later revision redefines or copies in the returned-document fixtures.
func SignatureDictionary(doc []byte) (num int, body string, err error) {
	vi := bytes.Index(doc, []byte("/Adobe.PPKLite"))
	if vi < 0 {
		return 0, "", errors.New("testpdf: no PPKLite dictionary")
	}
	hs := bytes.LastIndex(doc[:vi], []byte(" 0 obj"))
	if hs < 0 {
		return 0, "", errors.New("testpdf: the dictionary has no object header")
	}
	ns := bytes.LastIndexAny(doc[:hs], "\n\r ") + 1
	if num, err = strconv.Atoi(string(doc[ns:hs])); err != nil {
		return 0, "", fmt.Errorf("testpdf: signature header: %w", err)
	}
	oe := bytes.Index(doc[hs:], []byte("endobj"))
	if oe < 0 {
		return 0, "", errors.New("testpdf: the dictionary does not end")
	}
	return num, strings.TrimSpace(string(doc[hs+len(" 0 obj") : hs+oe])), nil
}
