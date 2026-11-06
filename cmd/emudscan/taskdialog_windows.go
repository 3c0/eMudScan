//go:build windows

package main

// A Windows task dialog: a question asked with an icon and buttons named after what
// they do ("Accept and run the fix", "Decline"), where a message box only has Yes and
// No. Where comctl32 has no task dialog, a message box asks the same question.

import (
	"encoding/binary"
	"runtime"
	"unsafe"
)

var pTaskDialogIndirect = modcomctl32.NewProc("TaskDialogIndirect")

// packed builds a structure of comctl32's task dialog, which has no padding
// (commctrl.h packs it to 1 byte).
type packed []byte

func (p *packed) u32(v uint32) { *p = binary.LittleEndian.AppendUint32(*p, v) }

func (p *packed) ptr(v uintptr) {
	if unsafe.Sizeof(v) == 8 {
		*p = binary.LittleEndian.AppendUint64(*p, uint64(v))
	} else {
		p.u32(uint32(v))
	}
}

// askWarning asks question, explained by text, with a warning icon and two buttons:
// yes (which does what is asked) and no, the default (Enter, Escape and the close
// button answer no). It reports whether yes was chosen.
func askWarning(owner uintptr, question, text, yes, no string) bool {
	const idYes, idNo = 100, 101
	if pTaskDialogIndirect.Find() == nil {
		title, q, content, y, n := utf16p(appName+simulationMark), utf16p(question), utf16p(text), utf16p(yes), utf16p(no)
		var buttons packed
		buttons.u32(idYes)
		buttons.ptr(uintptr(unsafe.Pointer(y)))
		buttons.u32(idNo)
		buttons.ptr(uintptr(unsafe.Pointer(n)))
		var c packed
		c.u32(0) // cbSize, set below
		c.ptr(owner)
		c.ptr(0)            // hInstance
		c.u32(0x8 | 0x1000) // TDF_ALLOW_DIALOG_CANCELLATION | TDF_POSITION_RELATIVE_TO_WINDOW
		c.u32(0)            // no common buttons
		c.ptr(uintptr(unsafe.Pointer(title)))
		c.ptr(0xFFFF) // TD_WARNING_ICON
		c.ptr(uintptr(unsafe.Pointer(q)))
		c.ptr(uintptr(unsafe.Pointer(content)))
		c.u32(2)
		c.ptr(uintptr(unsafe.Pointer(&buttons[0])))
		c.u32(idNo) // the default button
		c.u32(0)    // no radio buttons
		c.ptr(0)
		c.u32(0)
		for range 8 { // verification text, expanded information, its two labels, footer icon, footer, callback and its data
			c.ptr(0)
		}
		c.u32(320) // cxWidth, in dialog units: a little wider than Windows chooses, for a long text
		binary.LittleEndian.PutUint32(c, uint32(len(c)))
		var clicked int32
		hr, _, _ := pTaskDialogIndirect.Call(uintptr(unsafe.Pointer(&c[0])), uintptr(unsafe.Pointer(&clicked)), 0, 0)
		runtime.KeepAlive(buttons)
		runtime.KeepAlive([]*uint16{title, q, content, y, n})
		if hr == 0 {
			return clicked == idYes
		}
	}
	return msgBox(owner, question+"\n\n"+text, 0x4|0x30|0x100 /* YESNO|ICONWARNING|DEFBUTTON2 */) == 6 // IDYES
}
