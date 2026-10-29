//go:build windows

package main

// Win32 bindings, constants, structures and small helpers used by the window.

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var (
	moduser32   = sysDLL("user32.dll")
	modgdi32    = sysDLL("gdi32.dll")
	modcomctl32 = syscall.NewLazyDLL("comctl32.dll") // by name: version 6, chosen by the manifest (see dll_windows.go)
	moduxtheme  = sysDLL("uxtheme.dll")
	modshell32  = sysDLL("shell32.dll")
	modole32    = sysDLL("ole32.dll")

	pRegisterClassExW          = moduser32.NewProc("RegisterClassExW")
	pCreateWindowExW           = moduser32.NewProc("CreateWindowExW")
	pDefWindowProcW            = moduser32.NewProc("DefWindowProcW")
	pDestroyWindow             = moduser32.NewProc("DestroyWindow")
	pShowWindow                = moduser32.NewProc("ShowWindow")
	pUpdateWindow              = moduser32.NewProc("UpdateWindow")
	pGetMessageW               = moduser32.NewProc("GetMessageW")
	pTranslateMessage          = moduser32.NewProc("TranslateMessage")
	pDispatchMessageW          = moduser32.NewProc("DispatchMessageW")
	pIsDialogMessageW          = moduser32.NewProc("IsDialogMessageW")
	pIsChild                   = moduser32.NewProc("IsChild")
	pPostQuitMessage           = moduser32.NewProc("PostQuitMessage")
	pPostMessageW              = moduser32.NewProc("PostMessageW")
	pSendMessageW              = moduser32.NewProc("SendMessageW")
	pSetWindowPos              = moduser32.NewProc("SetWindowPos")
	pGetClientRect             = moduser32.NewProc("GetClientRect")
	pGetWindowRect             = moduser32.NewProc("GetWindowRect")
	pLoadCursorW               = moduser32.NewProc("LoadCursorW")
	pSetCursor                 = moduser32.NewProc("SetCursor")
	pLoadImageW                = moduser32.NewProc("LoadImageW")
	pMessageBoxW               = moduser32.NewProc("MessageBoxW")
	pSetWindowTextW            = moduser32.NewProc("SetWindowTextW")
	pGetWindowTextW            = moduser32.NewProc("GetWindowTextW")
	pGetWindowTextLengthW      = moduser32.NewProc("GetWindowTextLengthW")
	pEnableWindow              = moduser32.NewProc("EnableWindow")
	pIsWindowEnabled           = moduser32.NewProc("IsWindowEnabled")
	pSetForegroundWindow       = moduser32.NewProc("SetForegroundWindow")
	pGetDpiForWindow           = moduser32.NewProc("GetDpiForWindow")
	pGetDpiForSystem           = moduser32.NewProc("GetDpiForSystem")
	pSetProcessDpiAwarenessCtx = moduser32.NewProc("SetProcessDpiAwarenessContext")
	pOpenClipboard             = moduser32.NewProc("OpenClipboard")
	pEmptyClipboard            = moduser32.NewProc("EmptyClipboard")
	pSetClipboardData          = moduser32.NewProc("SetClipboardData")
	pCloseClipboard            = moduser32.NewProc("CloseClipboard")
	pCreateIconIndirect        = moduser32.NewProc("CreateIconIndirect")
	pDestroyIcon               = moduser32.NewProc("DestroyIcon")
	pDrawIconEx                = moduser32.NewProc("DrawIconEx")
	pGetSysColorBrush          = moduser32.NewProc("GetSysColorBrush")
	pGetSysColor               = moduser32.NewProc("GetSysColor")
	pGetSystemMetrics          = moduser32.NewProc("GetSystemMetrics")
	pGetSystemMetricsForDpi    = moduser32.NewProc("GetSystemMetricsForDpi")
	pSystemParametersInfoW     = moduser32.NewProc("SystemParametersInfoW")
	pSetFocus                  = moduser32.NewProc("SetFocus")
	pGetFocus                  = moduser32.NewProc("GetFocus")
	pSetCapture                = moduser32.NewProc("SetCapture")
	pReleaseCapture            = moduser32.NewProc("ReleaseCapture")
	pGetCursorPos              = moduser32.NewProc("GetCursorPos")
	pScreenToClient            = moduser32.NewProc("ScreenToClient")
	pClientToScreen            = moduser32.NewProc("ClientToScreen")
	pCreatePopupMenu           = moduser32.NewProc("CreatePopupMenu")
	pAppendMenuW               = moduser32.NewProc("AppendMenuW")
	pTrackPopupMenu            = moduser32.NewProc("TrackPopupMenu")
	pDestroyMenu               = moduser32.NewProc("DestroyMenu")
	pSetTimer                  = moduser32.NewProc("SetTimer")
	pKillTimer                 = moduser32.NewProc("KillTimer")
	pInvalidateRect            = moduser32.NewProc("InvalidateRect")
	pBeginPaint                = moduser32.NewProc("BeginPaint")
	pEndPaint                  = moduser32.NewProc("EndPaint")
	pFillRect                  = moduser32.NewProc("FillRect")
	pDrawTextW                 = moduser32.NewProc("DrawTextW")
	pGetDC                     = moduser32.NewProc("GetDC")
	pReleaseDC                 = moduser32.NewProc("ReleaseDC")
	pGetKeyState               = moduser32.NewProc("GetKeyState")
	pGetScrollInfo             = moduser32.NewProc("GetScrollInfo")
	pGetSaveFileNameW          = sysDLL("comdlg32.dll").NewProc("GetSaveFileNameW")
	pRedrawWindow              = moduser32.NewProc("RedrawWindow")
	pSetScrollInfo             = moduser32.NewProc("SetScrollInfo")
	pTrackMouseEvent           = moduser32.NewProc("TrackMouseEvent")
	pEnumChildWindows          = moduser32.NewProc("EnumChildWindows")
	pGetDoubleClickTime        = moduser32.NewProc("GetDoubleClickTime")
	pMonitorFromWindow         = moduser32.NewProc("MonitorFromWindow")
	pGetMonitorInfoW           = moduser32.NewProc("GetMonitorInfoW")

	pCreateFontW          = modgdi32.NewProc("CreateFontW")
	pDeleteObject         = modgdi32.NewProc("DeleteObject")
	pSelectObject         = modgdi32.NewProc("SelectObject")
	pCreateBitmap         = modgdi32.NewProc("CreateBitmap")
	pCreateDIBSection     = modgdi32.NewProc("CreateDIBSection")
	pSetBkMode            = modgdi32.NewProc("SetBkMode")
	pSetBkColor           = modgdi32.NewProc("SetBkColor")
	pSetTextColor         = modgdi32.NewProc("SetTextColor")
	pGetTextExtentPoint32 = modgdi32.NewProc("GetTextExtentPoint32W")
	pGetTextExtentExPoint = modgdi32.NewProc("GetTextExtentExPointW")
	pGetTextMetricsW      = modgdi32.NewProc("GetTextMetricsW")
	pCreateCompatibleDC   = modgdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBmp  = modgdi32.NewProc("CreateCompatibleBitmap")
	pBitBlt               = modgdi32.NewProc("BitBlt")
	pDeleteDC             = modgdi32.NewProc("DeleteDC")
	pExtTextOutW          = modgdi32.NewProc("ExtTextOutW")
	pCreateSolidBrush     = modgdi32.NewProc("CreateSolidBrush")
	pCreatePen            = modgdi32.NewProc("CreatePen")
	pGetStockObject       = modgdi32.NewProc("GetStockObject")
	pRoundRect            = modgdi32.NewProc("RoundRect")
	pMoveToEx             = modgdi32.NewProc("MoveToEx")
	pLineTo               = modgdi32.NewProc("LineTo")

	pInitCommonControlsEx = modcomctl32.NewProc("InitCommonControlsEx")
	pImageListCreate      = modcomctl32.NewProc("ImageList_Create")
	pImageListReplaceIcon = modcomctl32.NewProc("ImageList_ReplaceIcon")
	pImageListDestroy     = modcomctl32.NewProc("ImageList_Destroy")
	pSetWindowSubclass    = modcomctl32.NewProc("SetWindowSubclass")
	pDefSubclassProc      = modcomctl32.NewProc("DefSubclassProc")

	pSetWindowTheme = moduxtheme.NewProc("SetWindowTheme")

	pShellExecuteW              = modshell32.NewProc("ShellExecuteW")
	pSHOpenFolderAndSelectItems = modshell32.NewProc("SHOpenFolderAndSelectItems")
	pILCreateFromPathW          = modshell32.NewProc("ILCreateFromPathW")
	pILFree                     = modshell32.NewProc("ILFree")
	pCoInitializeEx             = modole32.NewProc("CoInitializeEx")

	pGetModuleHandleW = modkernel.NewProc("GetModuleHandleW")
	pGlobalAlloc      = modkernel.NewProc("GlobalAlloc")
	pGlobalLock       = modkernel.NewProc("GlobalLock")
	pGlobalUnlock     = modkernel.NewProc("GlobalUnlock")
	pGlobalFree       = modkernel.NewProc("GlobalFree")
)

const (
	wsOverlappedWindow = 0x00CF0000
	wsChild            = 0x40000000
	wsVisible          = 0x10000000
	wsTabStop          = 0x00010000
	wsGroup            = 0x00020000
	wsVScroll          = 0x00200000
	wsClipSiblings     = 0x04000000
	wsMinimizeBox      = 0x00020000
	wsExClientEdge     = 0x00000200
	wsExControlParent  = 0x00010000

	wmDestroy        = 0x0002
	wmSize           = 0x0005
	wmPaint          = 0x000F
	wmClose          = 0x0010
	wmEraseBkgnd     = 0x0014
	wmSetCursor      = 0x0020
	wmGetMinMaxInfo  = 0x0024
	wmSetFont        = 0x0030
	wmSetRedraw      = 0x000B
	wmNotify         = 0x004E
	wmContextMenu    = 0x007B
	wmCommand        = 0x0111
	wmTimer          = 0x0113
	wmVScroll        = 0x0115
	wmCtlColorStatic = 0x0138
	wmMouseMove      = 0x0200
	wmLButtonDown    = 0x0201
	wmLButtonUp      = 0x0202
	wmLButtonDblClk  = 0x0203
	wmRButtonUp      = 0x0205
	wmMouseWheel     = 0x020A
	wmMouseLeave     = 0x02A3
	wmKeyDown        = 0x0100
	wmSetFocus       = 0x0007
	wmKillFocus      = 0x0008
	wmGetDlgCode     = 0x0087
	wmCaptureChanged = 0x0215
	wmDpiChanged     = 0x02E0
	wmApp            = 0x8000
	wmAppEvents      = wmApp + 1

	bmGetCheck = 0x00F0
	bmSetCheck = 0x00F1

	lvmFirst            = 0x1000
	lvmSetImageList     = lvmFirst + 3
	lvmGetItemCount     = lvmFirst + 4
	lvmDeleteAllItems   = lvmFirst + 9
	lvmGetNextItem      = lvmFirst + 12
	lvmGetItemRect      = lvmFirst + 14
	lvmEnsureVisible    = lvmFirst + 19
	lvmScroll           = lvmFirst + 20
	lvmGetColumnWidth   = lvmFirst + 29
	lvmSetColumnWidth   = lvmFirst + 30
	lvmGetHeader        = lvmFirst + 31
	lvmGetTopIndex      = lvmFirst + 39
	lvmSetItemState     = lvmFirst + 43
	lvmGetItemState     = lvmFirst + 44
	lvmGetSelectedCount = lvmFirst + 50
	lvmSetExtendedStyle = lvmFirst + 54
	lvmGetSubItemRect   = lvmFirst + 56
	lvmSubItemHitTest   = lvmFirst + 57
	lvmApproxViewRect   = lvmFirst + 64
	lvmGetItemW         = lvmFirst + 75
	lvmSetItemW         = lvmFirst + 76
	lvmInsertItemW      = lvmFirst + 77
	lvmGetStringWidthW  = lvmFirst + 87
	lvmInsertColumnW    = lvmFirst + 97
	lvmSetItemTextW     = lvmFirst + 116
	lvmInsertGroup      = lvmFirst + 145
	lvmEnableGroupView  = lvmFirst + 157

	cbAddString = 0x0143
	cbGetCurSel = 0x0147
	cbSetCurSel = 0x014E

	cbGetDroppedState = 0x0157

	hdmGetItemW = 0x120B
	hdmSetItemW = 0x120C

	lvifText  = 0x1
	lvifImage = 0x2
	lvifParam = 0x4
	lvifState = 0x8

	nmClick        = -2
	nmDblClk       = -3
	nmReturn       = -4
	nmCustomDraw   = -12
	lvnColumnClick = -108
	lvnKeyDown     = -155
	hdnItemChanged = -321
	hdnEndTrack    = -327
	hdnDividerDbl  = -325

	idCheck   = 101
	idCopy    = 102
	idSave    = 103
	idLang    = 104
	idHideNet = 105
	idHideApp = 106
	idListWin = 107
	idListNet = 108
	idListApp = 109
	idLog     = 110
	idOnline  = 111
	idSearch  = 112
	idDrive0  = 150 // + drive index

	idDlgRich   = 301
	idDlgCopy   = 302
	idDlgCmds   = 303
	idDlgFolder = 304
	idDlgClose  = 305

	menuDetails = 201 + iota
	menuOpenFile
	menuOpenFolder
	menuHomepage
	menuSearch
	menuRecheck
	menuCopy
	menuCopyPaths
	menuSelectAll

	timerSpin = 1
	timerLang = 2
)

type wndClassEx struct {
	Size, Style                        uint32
	WndProc                            uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	IconSm                             uintptr
}

type msgT struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

type lvColumn struct {
	Mask      uint32
	Fmt       int32
	Cx        int32
	Text      *uint16
	TextMax   int32
	SubItem   int32
	Image     int32
	Order     int32
	CxMin     int32
	CxDefault int32
	CxIdeal   int32
}

type lvItem struct {
	Mask      uint32
	Item      int32
	SubItem   int32
	State     uint32
	StateMask uint32
	Text      *uint16
	TextMax   int32
	Image     int32
	LParam    uintptr
	Indent    int32
	GroupID   int32
	Columns   uint32
	PuColumns *uint32
	PiColFmt  *int32
	Group     int32
}

type lvGroup struct {
	Size           uint32
	Mask           uint32
	Header         *uint16
	CchHeader      int32
	Footer         *uint16
	CchFooter      int32
	GroupID        int32
	StateMask      uint32
	State          uint32
	Align          uint32
	Subtitle       *uint16
	CchSubtitle    uint32
	Task           *uint16
	CchTask        uint32
	DescTop        *uint16
	CchDescTop     uint32
	DescBottom     *uint16
	CchDescBottom  uint32
	TitleImage     int32
	ExtendedImage  int32
	FirstItem      int32
	Items          uint32
	SubsetTitle    *uint16
	CchSubsetTitle uint32
}

type nmhdr struct {
	HwndFrom uintptr
	IDFrom   uintptr
	Code     int32
}

type nmItemActivate struct {
	Hdr      nmhdr
	Item     int32
	SubItem  int32
	Pt       point
	NewState uint32
	OldState uint32
	Changed  uint32
	LParam   uintptr
	KeyFlags uint32
}

type nmListView struct {
	Hdr     nmhdr
	Item    int32
	SubItem int32
}

type nmLVKeyDown struct {
	Hdr   nmhdr
	VKey  uint16
	Flags uint32
}

type nmLVCustomDraw struct {
	Hdr        nmhdr
	DrawStage  uint32
	Hdc        uintptr
	Rc         rect
	ItemSpec   uintptr
	ItemState  uint32
	ItemLParam uintptr
	ClrText    uint32
	ClrTextBk  uint32
	SubItem    int32
}

type lvHitTestInfo struct {
	Pt      point
	Flags   uint32
	Item    int32
	SubItem int32
	Group   int32
}

type hdItem struct {
	Mask    uint32
	Cxy     int32
	Text    *uint16
	Hbm     uintptr
	TextMax int32
	Fmt     int32
	LParam  uintptr
	Image   int32
	Order   int32
	Type    uint32
	Filter  uintptr
	State   uint32
}

type scrollInfo struct {
	Size, Mask    uint32
	Min, Max      int32
	Page          uint32
	Pos, TrackPos int32
}

type textMetric struct {
	Height, Ascent, Descent, InternalLeading, ExternalLeading int32
	AveCharWidth, MaxCharWidth, Weight, Overhang              int32
	DigitizedAspectX, DigitizedAspectY                        int32
	FirstChar, LastChar, DefaultChar, BreakChar               uint16
	Italic, Underlined, StruckOut, PitchAndFamily, CharSet    byte
	_                                                         [3]byte
}

type toolInfo struct {
	Size     uint32
	Flags    uint32
	Hwnd     uintptr
	ID       uintptr
	Rect     rect
	Inst     uintptr
	Text     *uint16
	LParam   uintptr
	Reserved uintptr
}

type monitorInfo struct {
	Size          uint32
	Monitor, Work rect
	Flags         uint32
}

type paintStruct struct {
	Hdc  uintptr
	Rest [64]byte
}

type bitmapInfoHeader struct {
	Size                         uint32
	Width, Height                int32
	Planes, BitCount             uint16
	Compression, SizeImage       uint32
	XPelsPerMeter, YPelsPerMeter int32
	ClrUsed, ClrImportant        uint32
}

type iconInfoT struct {
	FIcon              int32
	XHotspot, YHotspot uint32
	HbmMask, HbmColor  uintptr
}

// ---------------------------------------------------------------------------
// small helpers

func u16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(strings.ReplaceAll(s, "\x00", ""))
	if err != nil {
		p, _ = syscall.UTF16PtrFromString("?")
	}
	return p
}

// up returns a pointer to a UTF-16 copy of s for a Windows call. The buffers of the
// last calls are kept referenced so that the garbage collector cannot free them
// while Windows is still reading them.
var (
	keepMu   sync.Mutex
	keepRing [512]*uint16
	keepNext int
)

func up(s string) uintptr {
	p := u16(s)
	keepMu.Lock()
	keepRing[keepNext] = p
	keepNext = (keepNext + 1) % len(keepRing)
	keepMu.Unlock()
	return uintptr(unsafe.Pointer(p))
}

func send(h uintptr, msg uint32, w, l uintptr) uintptr {
	r, _, _ := pSendMessageW.Call(h, uintptr(msg), w, l)
	return r
}

// ptr converts a pointer received from Windows (message parameters).
func ptr(u uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&u)) }

func loword(x uintptr) uint32 { return uint32(x) & 0xffff }
func hiword(x uintptr) uint32 { return uint32(x) >> 16 & 0xffff }
func xParam(l uintptr) int32  { return int32(int16(uint16(l))) }
func yParam(l uintptr) int32  { return int32(int16(uint16(l >> 16))) }

func sysColor(i int) uint32 {
	c, _, _ := pGetSysColor.Call(uintptr(i))
	return uint32(c)
}

func rgb(r, g, b byte) uint32 { return uint32(r) | uint32(g)<<8 | uint32(b)<<16 }

func setText(h uintptr, s string) { pSetWindowTextW.Call(h, up(s)) }

func getText(h uintptr) string {
	n, _, _ := pGetWindowTextLengthW.Call(h)
	buf := make([]uint16, n+1)
	pGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

func msgBox(owner uintptr, text string, flags uintptr) uintptr {
	r, _, _ := pMessageBoxW.Call(owner, up(text), up(appName), flags)
	return r
}

func ctrlDown() bool {
	r, _, _ := pGetKeyState.Call(0x11)
	return r&0x8000 != 0
}

func shiftDown() bool {
	r, _, _ := pGetKeyState.Call(0x10)
	return r&0x8000 != 0
}

func setClipboard(owner uintptr, text string) bool {
	u := utf16z(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n"))
	if r, _, _ := pOpenClipboard.Call(owner); r == 0 {
		return false
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	h, _, _ := pGlobalAlloc.Call(0x0002 /* GMEM_MOVEABLE */, uintptr(len(u)*2))
	if h == 0 {
		return false
	}
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		pGlobalFree.Call(h)
		return false
	}
	copy(unsafe.Slice((*uint16)(ptr(p)), len(u)), u)
	pGlobalUnlock.Call(h)
	if r, _, _ := pSetClipboardData.Call(13 /* CF_UNICODETEXT */, h); r == 0 {
		pGlobalFree.Call(h)
		return false
	}
	return true
}

func iconFromRGBA(px []byte, size int) uintptr { return iconFromRGBAWH(px, size, size) }

// iconFromRGBAWH makes an icon of w x h pixels.
func iconFromRGBAWH(px []byte, w, h int) uintptr {
	bih := bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	var bits unsafe.Pointer
	color, _, _ := pCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bih)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if color == 0 || bits == nil {
		return 0
	}
	dst := unsafe.Slice((*byte)(bits), w*h*4)
	for i := 0; i < w*h; i++ {
		dst[i*4+0] = px[i*4+2]
		dst[i*4+1] = px[i*4+1]
		dst[i*4+2] = px[i*4+0]
		dst[i*4+3] = px[i*4+3]
	}
	// the AND mask marks the transparent pixels, for the drawings that ignore alpha
	// (a disabled button, for instance)
	stride := (w + 15) / 16 * 2
	mbits := make([]byte, stride*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if px[(y*w+x)*4+3] < 128 {
				mbits[y*stride+x/8] |= 0x80 >> (x % 8)
			}
		}
	}
	mask, _, _ := pCreateBitmap.Call(uintptr(w), uintptr(h), 1, 1, uintptr(unsafe.Pointer(&mbits[0])))
	ii := iconInfoT{FIcon: 1, HbmMask: mask, HbmColor: color}
	icon, _, _ := pCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	pDeleteObject.Call(color)
	pDeleteObject.Call(mask)
	return icon
}

func createFont(pt, dpi, weight int, underline bool, face string) uintptr {
	h := -(pt*dpi + 36) / 72
	ul := uintptr(0)
	if underline {
		ul = 1
	}
	f, _, _ := pCreateFontW.Call(uintptr(int32(h)), 0, 0, 0, uintptr(weight), 0, ul, 0, 1, 0, 0, 5, 0, up(face))
	return f
}

// openPath opens Explorer: a folder is opened, a file is selected in its folder.
func openPath(owner uintptr, p string) {
	if p == "" {
		return
	}
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		pidl, _, _ := pILCreateFromPathW.Call(up(p))
		if pidl != 0 {
			r, _, _ := pSHOpenFolderAndSelectItems.Call(pidl, 0, 0, 0)
			pILFree.Call(pidl)
			if r == 0 {
				return
			}
		}
		p = filepath.Dir(p)
	}
	pShellExecuteW.Call(owner, up("open"), up(p), 0, 0, 1)
}

func openURL(owner uintptr, u string) {
	if strings.HasPrefix(lower(u), "http://") || strings.HasPrefix(lower(u), "https://") {
		pShellExecuteW.Call(owner, up("open"), up(u), 0, 0, 1)
	}
}

// openTarget opens a link of the markup: a URL or a path.
func openTarget(owner uintptr, t string) {
	if rest, ok := strings.CutPrefix(t, openInPrefix); ok { // a page in a given browser
		if exe, u, ok := strings.Cut(rest, ">"); ok {
			pShellExecuteW.Call(owner, up("open"), up(exe), up(u), 0, 1)
		}
		return
	}
	if strings.HasPrefix(lower(t), "http") {
		openURL(owner, t)
	} else {
		openPath(owner, t)
	}
}

type openFileName struct {
	StructSize    uint32
	Owner         uintptr
	Instance      uintptr
	Filter        *uint16
	CustomFilter  *uint16
	MaxCustFilter uint32
	FilterIndex   uint32
	File          *uint16
	MaxFile       uint32
	FileTitle     *uint16
	MaxFileTitle  uint32
	InitialDir    *uint16
	Title         *uint16
	Flags         uint32
	FileOffset    uint16
	FileExtension uint16
	DefExt        *uint16
	CustData      uintptr
	FnHook        uintptr
	TemplateName  *uint16
	PvReserved    uintptr
	DwReserved    uint32
	FlagsEx       uint32
}

// utf16z converts s to a NUL-terminated UTF-16 string, dropping any NUL inside
// (syscall.StringToUTF16 panics on them).
func utf16z(s string) []uint16 { return syscall.StringToUTF16(strings.ReplaceAll(s, "\x00", "")) }
