package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// wndClassExW https://msdn.microsoft.com/en-us/library/windows/desktop/ms633577.aspx
type wndClassExW struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   syscall.Handle
	icon       syscall.Handle
	cursor     syscall.Handle
	background syscall.Handle
	menuName   *uint16
	className  *uint16
	iconSm     syscall.Handle
}

const CS_HREDRAW = 0x0002
const CS_VREDRAW = 0x0001
const WS_OVERLAPPEDWINDOW = 0x00000000 | 0x00C00000 | 0x00080000 | 0x00040000 | 0x00020000 | 0x00010000

var clipboard_hwnd uintptr
var hWndNewNext uintptr
var clipText = make([]byte, 0x1000)

func MAKEINTRESOURCEA(id uint16) *byte {
	return (*byte)(unsafe.Pointer(uintptr(id)))
}

func create_window(class_name uintptr, window_name uintptr, callback uintptr) uintptr {

	var wnd_class wndClassExW
	var hInstance uintptr

	hInstance = 0xffffffffffffffff
	wnd_class.size = uint32(unsafe.Sizeof(wndClassExW{}))
	wnd_class.style = CS_HREDRAW | CS_VREDRAW
	wnd_class.wndProc = callback
	wnd_class.clsExtra = 0
	wnd_class.wndExtra = 0
	wnd_class.instance = syscall.Handle(hInstance)
	r, _, _ := loadIconA.Call(0, uintptr(unsafe.Pointer(MAKEINTRESOURCEA(32512))))
	wnd_class.icon = syscall.Handle(r)
	r1, _, _ := loadCursorA.Call(0, uintptr(unsafe.Pointer(MAKEINTRESOURCEA(32512))))
	wnd_class.cursor = syscall.Handle(r1)
	wnd_class.background = 6
	wnd_class.menuName = (*uint16)(unsafe.Pointer(uintptr(0)))
	wnd_class.className = (*uint16)(unsafe.Pointer(class_name))
	wnd_class.iconSm = 0
	RegisterClassExW.Call(uintptr(unsafe.Pointer(&wnd_class)))
	r2, _, _ := CreateWindowExW.Call(0, class_name, window_name, WS_OVERLAPPEDWINDOW, 0x80000000, 0, 0x80000000, 0, 0, 0, hInstance, 0)

	return r2

}

var (
	user32                     = syscall.NewLazyDLL("user32.dll")
	procSetClipboardViewer     = user32.NewProc("SetClipboardViewer")
	procChangeClipboardChain   = user32.NewProc("ChangeClipboardChain")
	procGetClipboardData       = user32.NewProc("GetClipboardData")
	procOpenClipboard          = user32.NewProc("OpenClipboard")
	procCloseClipboard         = user32.NewProc("CloseClipboard")
	loadIconA                  = user32.NewProc("LoadIconA")
	loadCursorA                = user32.NewProc("LoadCursorA")
	RegisterClassExW           = user32.NewProc("RegisterClassExW")
	CreateWindowExW            = user32.NewProc("CreateWindowExW")
	procSendMessage            = user32.NewProc("SendMessageA")
	getPriorityClipboardFormat = user32.NewProc("GetPriorityClipboardFormat")
	defWndProc                 = user32.NewProc("DefWindowProcA")
	procTranslateMessage       = user32.NewProc("TranslateMessage")
	procDispatchMessage        = user32.NewProc("DispatchMessageW")
	GetMessage                 = user32.NewProc("GetMessageW")

	k32              = syscall.NewLazyDLL("kernel32.dll")
	procGlobalSize   = k32.NewProc("GlobalSize")
	procGlobalLock   = k32.NewProc("GlobalLock")
	procGlobalUnlock = k32.NewProc("GlobalUnlock")
)

const (
	CF_TEXT          = 1
	WM_CREATE        = 0x0001
	WM_DESTROY       = 0x0002
	WM_DRAWCLIPBOARD = 0x0308
	WM_CHANGECBCHAIN = 0x030D
)

func clipwndCallback(hWndNewViewer syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_CREATE:
		hWndNewNext, _, _ = procSetClipboardViewer.Call(uintptr(hWndNewViewer))

	case WM_CHANGECBCHAIN:
		if wParam == hWndNewNext {
			hWndNewNext = lParam
		} else if hWndNewNext != 0 {
			procSendMessage.Call(uintptr(hWndNewNext), uintptr(msg), wParam, lParam)
		}

	case WM_DRAWCLIPBOARD:
		if hWndNewNext != 0 {
			procSendMessage.Call(uintptr(hWndNewNext), uintptr(msg), wParam, lParam)
		}

		var priorityList = []uint32{CF_TEXT}

		hMem := uintptr(0)
		gpcf, _, _ := getPriorityClipboardFormat.Call(uintptr(unsafe.Pointer(&priorityList[0])), 1)

		if gpcf == CF_TEXT {
			r, _, _ := procOpenClipboard.Call(uintptr(hWndNewViewer))
			if r != 0 {
				hMem, _, _ = procGetClipboardData.Call(CF_TEXT)
				if hMem != 0 {
					size, _, _ := procGlobalSize.Call(hMem)
					pMem, _, _ := procGlobalLock.Call(hMem)
					copySize := uintptr(0)
					if size < 4096 {
						copySize = size
					} else {
						copySize = 4095
					}
					clipText = (*[4096]byte)(unsafe.Pointer(pMem))[:copySize:copySize]
					clipText = append(clipText, []byte{0x00}[0])
					procGlobalUnlock.Call(hMem)
					procCloseClipboard.Call()

					gbktext, e := GbkToUtf8(clipText)
					if e == nil {
						fmt.Println(string(gbktext))
					} else {
						fmt.Println(string(clipText))
					}

				}
			}

		}

	case WM_DESTROY:
		if hWndNewViewer != 0 {
			procChangeClipboardChain.Call(uintptr(hWndNewViewer), uintptr(hWndNewNext))
		}

	default:
		break

	}
	r, _, _ := defWndProc.Call(uintptr(hWndNewViewer), uintptr(msg), wParam, lParam)
	return r

}

func GbkToUtf8(s []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(s), simplifiedchinese.GBK.NewDecoder())
	d, e := ioutil.ReadAll(reader)
	if e != nil {
		return nil, e
	}
	return d, nil
}

func exec_clipboard_mon() {
	if clipboard_hwnd == 0 {
		classname, _ := syscall.UTF16PtrFromString("clipboard_monitor_class")
		windowname, _ := syscall.UTF16PtrFromString("clipboard_monitor_name")
		clipwnd_callback := syscall.NewCallback(clipwndCallback)

		clipboard_hwnd = create_window((uintptr)(unsafe.Pointer(classname)), (uintptr)(unsafe.Pointer(windowname)), clipwnd_callback)
	}
	if clipboard_hwnd == 0 {
		fmt.Println("create window error")
		os.Exit(1)
	}
}

type MSG struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

type POINT struct {
	X, Y int32
}

func TranslateMessage(msg *MSG) uintptr {
	ret, _, _ := procTranslateMessage.Call(uintptr(unsafe.Pointer(msg)))
	return ret
}
func DispatchMessage(msg *MSG) uintptr {
	ret, _, _ := procDispatchMessage.Call(uintptr(unsafe.Pointer(msg)))
	return ret
}

func main() {
	var msg MSG
	exec_clipboard_mon()

	for {
		bRet, _, _ := GetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(bRet) <= 0 {
			fmt.Println("GetMessage error")
			return
		}
		TranslateMessage(&msg)
		DispatchMessage(&msg)
	}

}
