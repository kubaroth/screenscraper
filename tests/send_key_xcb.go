// Spends Page-down key event to a window which contains name "Chrome"
package main

import (
	"fmt"
	"time"
	"log"
	"strings"

	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgb/xtest"

	"github.com/BurntSushi/xgbutil"
	"github.com/BurntSushi/xgbutil/ewmh"

)


func getWindowId(X *xgbutil.XUtil, name string) (xproto.Window){

	// Get a list of all client ids.
	clientids, err := ewmh.ClientListGet(X)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(clientids)

	var destination_window  xproto.Window;
	for _, clientid := range clientids {
		window_name, _ := ewmh.WmNameGet(X, clientid)
		fmt.Println(name, clientid)
		if strings.Contains(window_name, name) == true{
			fmt.Println(destination_window);
			fmt.Println("destination_window:", name);
			destination_window = clientid
		}
		if destination_window == 0{
			fmt.Println("No window found", destination_window)
			// TODO return 
		}
	}
	return destination_window
}

func bringWindowAbove(X *xgbutil.XUtil, destination_window xproto.Window){
	// NOTE: Using a workaround.
	// Instead of just ewmh.ActiveWindowSet which has no effect.
	// We set Focus which then receives key events
	xproto.SetInputFocus(X.Conn(), xproto.InputFocusParent, destination_window, xproto.TimeCurrentTime)

	time.Sleep(2000 * time.Millisecond)
	fmt.Println("active window is now", destination_window)

	ewmh.WmStateReq(X, destination_window, ewmh.StateToggle, "_NET_WM_STATE_ABOVE")
}

func disableWindowAbove(X *xgbutil.XUtil, destination_window xproto.Window){
	ewmh.WmStateReq(X, destination_window, ewmh.StateRemove, "_NET_WM_STATE_ABOVE")
}

func nextPage(X *xgbutil.XUtil, destination_window xproto.Window) {
	PAGE_DOWN      := 117

	// Press key
	xtest.FakeInput(X.Conn(),
		xproto.KeyPress,
		byte(PAGE_DOWN), 
		xproto.TimeCurrentTime,
		destination_window,
		0,0,
		0)


	// Release key
	xtest.FakeInput(X.Conn(),
		xproto.KeyRelease,
		byte(PAGE_DOWN), 
		xproto.TimeCurrentTime,
		destination_window,
		0,0,
		0)

	time.Sleep(2000 * time.Millisecond)
}

func main(){
	X, err := xgbutil.NewConn()
	if err != nil {
		log.Fatal(err)
	}

	xtest.Init(X.Conn())

	destination_window := getWindowId(X, "Chrome") 

	bringWindowAbove(X, destination_window)

	nextPage(X, destination_window)
	time.Sleep(2000 * time.Millisecond)
	nextPage(X, destination_window)
	time.Sleep(2000 * time.Millisecond)

	// Once done remove this property
	disableWindowAbove(X, destination_window)
}
