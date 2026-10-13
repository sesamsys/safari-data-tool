package sizer_test

import (
	"fmt"
	"math/rand"
	"testing"
	"testing/quick"

	"github.com/sesamsys/safari-data-tool/internal/fsx"
	"github.com/sesamsys/safari-data-tool/internal/sizer"
)

// ceilKB rounds a byte count up to the next whole 1024-byte block, mirroring
// the per-file ceiling that SizeKB applies.
func ceilKB(bytes int64) int64 { return (bytes + 1023) / 1024 }

// buildSubtree seeds a single top-level subtree under the fake root with a
// random number of regular files of random sizes, nested at random depths. It
// returns the independently computed KB total for that subtree.
//
// Files are placed only inside subtree directories (never directly at the fake
// root) so the subtree partition is clean: every regular file in the tree
// belongs to exactly one top-level subtree.
func buildSubtree(t *testing.T, f *fsx.FakeFS, rng *rand.Rand, subtreeIdx int) int64 {
	t.Helper()

	var wantKB int64
	fileCount := rng.Intn(6) // 0..5 files per subtree

	for i := 0; i < fileCount; i++ {
		// Random nesting depth 0..3 within this subtree.
		rel := fmt.Sprintf("sub%d", subtreeIdx)
		depth := rng.Intn(4)
		for d := 0; d < depth; d++ {
			rel += fmt.Sprintf("/d%d", rng.Intn(3))
		}
		rel += fmt.Sprintf("/file%d", i)

		// Random file size 0..5000 bytes exercises the ceiling across
		// several block boundaries (including the empty-file 0 KB case).
		size := rng.Intn(5001)
		data := make([]byte, size)
		if _, err := f.WriteFile(rel, data); err != nil {
			t.Fatalf("WriteFile(%q): %v", rel, err)
		}
		wantKB += ceilKB(int64(size))
	}

	// Ensure the subtree directory exists even when it has no files, so the
	// walk still encounters it as an empty subtree.
	if fileCount == 0 {
		if _, err := f.Mkdir(fmt.Sprintf("sub%d", subtreeIdx)); err != nil {
			t.Fatalf("Mkdir sub%d: %v", subtreeIdx, err)
		}
	}

	return wantKB
}

// TestSizeKB_RecursiveAdditivity exercises feature "safari-data-tool"
// Property 7: Recursive size is additive.
//
// For any directory tree, SizeKB(root) equals an independently summed total of
// each regular file's ceil(bytes/1024) KB, and the sum of SizeKB over a
// partition of the tree into top-level subtrees equals SizeKB(root).
//
// **Validates: Requirements 3.1**
func TestSizeKB_RecursiveAdditivity(t *testing.T) {
	// property is driven by testing/quick with a seed so random file counts,
	// sizes, and nesting vary across >= 100 iterations.
	property := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		fake := fsx.NewFakeFS(t.TempDir())

		// Partition the tree into a random number (1..5) of top-level
		// subtrees. All files live inside these subtrees so none sit directly
		// at the fake root.
		subtreeCount := rng.Intn(5) + 1
		var wantTotalKB int64
		subtreeWantKB := make([]int64, subtreeCount)
		for i := 0; i < subtreeCount; i++ {
			subtreeWantKB[i] = buildSubtree(t, fake, rng, i)
			wantTotalKB += subtreeWantKB[i]
		}

		// 1. Whole-tree size equals the independently computed total.
		gotTotalKB, err := sizer.SizeKB(fake, fake.Root())
		if err != nil {
			t.Fatalf("SizeKB(root): %v", err)
		}
		if gotTotalKB != wantTotalKB {
			t.Errorf("SizeKB(root) = %d KB, want %d KB (seed=%d)", gotTotalKB, wantTotalKB, seed)
			return false
		}

		// 2. Additivity: the sum of subtree sizes equals the whole-tree size.
		var sumSubtreesKB int64
		for i := 0; i < subtreeCount; i++ {
			gotSubtreeKB, err := sizer.SizeKB(fake, fake.Path(fmt.Sprintf("sub%d", i)))
			if err != nil {
				t.Fatalf("SizeKB(sub%d): %v", i, err)
			}
			if gotSubtreeKB != subtreeWantKB[i] {
				t.Errorf("SizeKB(sub%d) = %d KB, want %d KB (seed=%d)", i, gotSubtreeKB, subtreeWantKB[i], seed)
				return false
			}
			sumSubtreesKB += gotSubtreeKB
		}
		if sumSubtreesKB != gotTotalKB {
			t.Errorf("sum of subtree sizes = %d KB, want whole-tree %d KB (seed=%d)", sumSubtreesKB, gotTotalKB, seed)
			return false
		}

		return true
	}

	// Minimum 100 iterations.
	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(property, cfg); err != nil {
		t.Fatal(err)
	}
}

// TestSizeKB_EmptyTree is a focused unit case: an empty tree sizes to 0 KB.
func TestSizeKB_EmptyTree(t *testing.T) {
	fake := fsx.NewFakeFS(t.TempDir())
	got, err := sizer.SizeKB(fake, fake.Root())
	if err != nil {
		t.Fatalf("SizeKB: %v", err)
	}
	if got != 0 {
		t.Errorf("SizeKB(empty) = %d KB, want 0 KB", got)
	}
}

// TestSizeKB_PerFileCeiling is a focused unit case covering the per-file block
// rounding boundaries: 0 -> 0, 1 -> 1, 1024 -> 1, 1025 -> 2.
func TestSizeKB_PerFileCeiling(t *testing.T) {
	cases := []struct {
		size   int
		wantKB int64
	}{
		{0, 0},
		{1, 1},
		{1023, 1},
		{1024, 1},
		{1025, 2},
		{2048, 2},
		{2049, 3},
	}
	for _, tc := range cases {
		fake := fsx.NewFakeFS(t.TempDir())
		if _, err := fake.WriteFile("f", make([]byte, tc.size)); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		got, err := sizer.SizeKB(fake, fake.Root())
		if err != nil {
			t.Fatalf("SizeKB: %v", err)
		}
		if got != tc.wantKB {
			t.Errorf("SizeKB(file of %d bytes) = %d KB, want %d KB", tc.size, got, tc.wantKB)
		}
	}
}
