// Package doctor checks backlog invariants and offers a fix path.
//
// It validates filename/ID integrity, uniqueness, index-detail consistency, dep
// integrity, cycles, archive location-field agreement, and attachment links.
// Fix repairs every losslessly fixable finding; findings that would change
// ticket identity (bad filenames, duplicates, unparseable details, cycles) are
// left for a human and re-reported.
package doctor
