package kind

// SourceExpectation is checked in the same room critical section as a merge.
// Epoch identifies the history; Frames contains only whole-file comparisons.
type SourceExpectation struct {
	Epoch  string
	Frames map[string]FrameExpectation
}

// FrameExpectation distinguishes authoritative absence from present source.
type FrameExpectation struct {
	Absent bool
	Source string
}

// SourceFile is one file of a live room and the content beside it, read with
// the room's epoch in one critical section, so a proof made from it is as
// coherent as one made from the whole bundle.
type SourceFile struct {
	Content string
	Text    string
	Present bool
	Epoch   string
}
