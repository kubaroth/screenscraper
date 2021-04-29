// A tool to automate screen-capturing long web pages and turn them into a Pdf.
// Usage:
// Run a tool, select a region to capture
// The script takes a screen-shot and advance to the next fragment of the page.
// Once the end of the page is reached, all captured images are converted to the Pdf
// NOTE:
// Before first run you need to change permission of /dev/uinput
//     sudo chmod +0666 /dev/uinput
// NOTE:
// Linux support only at the moment as it heavily relies on (great!) xcb package
package main

 import (
    "github.com/kbinani/screenshot"
    "image/png"
    "image"
	"image/color"
    "os"
    "fmt"
    "log"
    "time"

    "github.com/BurntSushi/xgbutil"
    "github.com/BurntSushi/xgbutil/ewmh"
    "github.com/BurntSushi/xgbutil/xwindow"

    "github.com/BurntSushi/xgbutil/mousebind"
    "github.com/BurntSushi/xgbutil/xevent"

    "github.com/BurntSushi/xgb/xproto"

    "github.com/BurntSushi/xgbutil/xgraphics"  // painting
    "github.com/BurntSushi/xgb"  // active window


    "github.com/micmonay/keybd_event"
    "runtime"
    "regexp"

    "github.com/jung-kurt/gofpdf"
    "path/filepath"
 )

// This function returns the name of the current active window
// Currently it is only used to pragmatically name resulted screenshots
func getActiveWindowName() (string) {

    X, err := xgb.NewConn()
    if err != nil {
        log.Fatal(err)
    }

    // Get the window id of the root window.
    setup := xproto.Setup(X)
    root := setup.DefaultScreen(X).Root

    // Get the atom id (i.e., intern an atom) of "_NET_ACTIVE_WINDOW".
    aname := "_NET_ACTIVE_WINDOW"
    activeAtom, err := xproto.InternAtom(X, true, uint16(len(aname)),
        aname).Reply()
    if err != nil {
        log.Fatal(err)
    }

    // Get the atom id (i.e., intern an atom) of "_NET_WM_NAME".
    aname = "_NET_WM_NAME"
    nameAtom, err := xproto.InternAtom(X, true, uint16(len(aname)),
        aname).Reply()
    if err != nil {
        log.Fatal(err)
    }

    // Get the actual value of _NET_ACTIVE_WINDOW.
    // Note that 'reply.Value' is just a slice of bytes, so we use an
    // XGB helper function, 'Get32', to pull an unsigned 32-bit integer out
    // of the byte slice. We then convert it to an X resource id so it can
    // be used to get the name of the window in the next GetProperty request.
    reply, err := xproto.GetProperty(X, false, root, activeAtom.Atom,
        xproto.GetPropertyTypeAny, 0, (1<<32)-1).Reply()
    if err != nil {
        log.Fatal(err)
    }
    windowId := xproto.Window(xgb.Get32(reply.Value))
    fmt.Printf("Active window id: %X\n", windowId)

    // Now get the value of _NET_WM_NAME for the active window.
    // Note that this time, we simply convert the resulting byte slice,
    // reply.Value, to a string.
    reply, err = xproto.GetProperty(X, false, windowId, nameAtom.Atom,
        xproto.GetPropertyTypeAny, 0, (1<<32)-1).Reply()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Active window name: %s\n", string(reply.Value))


    // Store the name of the window
    // Replace non-matching characters which can cause problems in file names
    re := regexp.MustCompile(`[^a-zA-Z0_9]`)
    window_name_to_focus := string(re.ReplaceAll([]byte(reply.Value), []byte("")))

    return window_name_to_focus

}


func max(a, b int) int {
    if a > b {
        return a
    }
    return b
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}
// midRect takes an (x, y) position where the pointer was clicked, along with
// the width and height of the thing being drawn and the width and height of
// the canvas, and returns a Rectangle whose midpoint (roughly) is (x, y) and
// whose width and height match the parameters when the rectangle doesn't
// extend past the border of the canvas. Make sure to check if the rectangle is
// empty or not before using it!
func midRect(x, y, width, height, canWidth, canHeight int) image.Rectangle {
    val:= image.Rect(
        max(0, min(canWidth, x-width/2)),   // top left x
        max(0, min(canHeight, y-height/2)), // top left y
        max(0, min(canWidth, x+width/2)),   // bottom right x
        max(0, min(canHeight, y+height/2)), // bottom right y
    )
    // fmt.Println(val)

    return val
}


// This method currenlty just paints the brush stroke.
// Start/End represent Bounding box corners
// TODO: draw real area for better preview
func drawRect(canvas *xgraphics.Image, win *xwindow.Window, x,y int){

    bg := xgraphics.BGRA{0x0, 0x0, 0x0, 0xff}; _ = bg
    pencil := xgraphics.BGRA{0xaa, 0x0, 0xff, 0x55};
    pencilTip := 10
    width := 1600
    height := 900 // TODO - get Bounds of the Monitor instead
    tipRect := midRect(x, y, pencilTip, pencilTip, width, height); _=tipRect

    // If the rectangle contains no pixels, don't draw anything.
    if tipRect.Empty() {
        return
    }

    // Create the subimage of the canvas to draw to.
    tip := canvas.SubImage(tipRect).(*xgraphics.Image)


    // Now color each pixel in tip with the pencil color.
    tip.For(func(x, y int) xgraphics.BGRA {
        return xgraphics.BlendBGRA(tip.At(x, y).(xgraphics.BGRA), pencil)
    })

    // Now draw the changes to the pixmap.
    tip.XDraw()

    // And paint them to the window.
    tip.XPaint(win.Id)
}


// Function to allow user input and select area of the screen to capture
// Currenlty the bounding box is represented by paint stroke where
// start (mouse-down) represents top-left corner
// release (mouse-up) represents bottom-right corner
// Release of the mouse button closes the X window and let program continue
// but reuquires to switch active window to the one we want to capture.
func getCaptureArea() (rect image.Rectangle) {

    if runtime.GOOS != "linux" {
        bounds := screenshot.GetDisplayBounds(0)  // Display: 0
        return bounds
    }

    // XCB - version - determin bounds with User's input
    X, err := xgbutil.NewConn()
    if err != nil {
        log.Fatal(err)
    }
    mousebind.Initialize(X)


    // Capture the state of the current screen - this workarounds the problem
    // of dealing with opacities which may require the feature to be enabled in compositor
    canvas, _ := xgraphics.NewDrawable(X, xproto.Drawable(X.RootWin()))

    bounds := image.Rect(0,0,canvas.Rect.Dx(),canvas.Rect.Dy())

    win := canvas.XShowExtra("Select area to caputre", true)

    // Once initialized turn to fullscreen (f11) to match coordinates on screen
    ewmh.WmStateReq(canvas.X, win.Id, ewmh.StateToggle, "_NET_WM_STATE_FULLSCREEN")

        err = mousebind.ButtonPressFun(
        func(X *xgbutil.XUtil, e xevent.ButtonPressEvent) {
            log.Println("A second handler always happens after the first.")
        }).Connect(X, X.RootWin(), "1", false, true)


    mousebind.Drag(X, X.RootWin(), X.RootWin(), "1", false,
        func(X *xgbutil.XUtil, rx, ry, ex, ey int) (bool, xproto.Cursor) {
            log.Println("starting", rx, ry)
            bounds.Min.X = rx
            bounds.Min.Y = ry
            return true, 0
        },
        func(X *xgbutil.XUtil, rx, ry, ex, ey int) {
            log.Println("pressed", rx, ry)
            drawRect(canvas, win , rx, ry)

        },
        func(X *xgbutil.XUtil, rx, ry, ex, ey int) {
            log.Println("release", rx, ry)
            bounds.Max.X = rx
            bounds.Max.Y = ry

            // gracefully exit
            xevent.Detach(win.X, win.Id)
            mousebind.Detach(win.X, win.Id)
            win.Destroy()
            xevent.Quit(X)
        })

    if err != nil {
        log.Fatal(err)
    }

    log.Println("Program initialized. Start pressing mouse buttons!")

    // Record the area to process
    xevent.Main(X)

    // Continue with other stuff
    fmt.Println("Bounds to process", bounds)

    return bounds
}

func capture_image(x,y,w,h int) (*image.RGBA){
    var img *image.RGBA
    // bounds := image.Rect(x, y, x+w, y+h)
    // // fmt.Println(bounds)
    // img, err := screenshot.CaptureRect(bounds)
    // if err != nil {
    //     panic(err)
    // }


    X, err := xgbutil.NewConn()
    if err != nil {
        log.Fatal(err)
    }

    canvas, _ := xgraphics.NewDrawable(X, xproto.Drawable(X.RootWin()))

    bounds := image.Rect(0,0,w,h)
	img = image.NewRGBA(bounds)

	for jy := 0; jy < h; jy++{
		for ix := 0; ix < w; ix++{
			// x,y are the top left corner of the area to caputure - we are indexing into it
			// canvas.Stride is the width of the current screen
			index := ((jy + y) * canvas.Stride + (ix + x)*4 )
			// fmt.Println(ix, jy, index)
			R := canvas.Pix[index]
			G := canvas.Pix[index+1]
			B := canvas.Pix[index+2]
			A := canvas.Pix[index+3]
			img.Set(ix,jy, color.RGBA{B,G,R,A})
		}
	}
    return img
}

// Compare previous and current image - return false if they don't match
func diff_images(img1 *image.RGBA, img2 *image.RGBA) bool{

    // skip if img2 is still not initialized
    if img2 == nil {
        // fmt.Println("second image is still nil")
        ret := false
        return ret
    }

    // compare bounds first
    if img2.Bounds() != img1.Bounds() {
        return false  // not the same, continue
    }
    for x:= 0; x < img1.Bounds().Max.X; x++{
        for y:= 0; y < img1.Bounds().Max.Y; y++ {
            if img1.RGBAAt(x,y) != img2.RGBAAt(x,y){
                return false
            }
        }
    }
    return true
}

func main() {
	// // img := capture_image(0,64,34, 20)
	// img := capture_image(0,0,500,500)
	// if img == nil{
	// 	fmt.Println("returning no screen capture taken")
	// 	return
	// }
	// file, _ := os.Create("/tmp/aaa.png")
	// // TODO: invoking the tool with OS short - needs to set up path correctly
	// if err := png.Encode(file, img); err != nil{
	// 	fmt.Printf("error encoding %s\n", err)
	// }




	info, err := os.Stat("/dev/uinput")
	m := info.Mode()

	fmt.Println(m.Perm().String())
	if m.Perm().String() != "-rw-rw-rw-"{
		fmt.Println("Keyboard input won't work - please change permissions:  sudo chmod +0666 /dev/uinput" )
		return
	}
	
    bounds := getCaptureArea()

    // NOTE: Focus on the target window is assumed here (This is no problem)
    // if the program is executed using the shortcut but if started from
    // terminal it will try to update terminal instead Chrome

    // TODO: need to grab focus on the specific Window (Chrome in this case)
    // In order to fire off go to the next page key action
    // NOTE: maybe this is not necessary as in most cases we will run program
    // from shortcut and the target window will already be in focus


    // Introduce some delay as we want to switch active window from the terminal
    // Note this is not the problem if we launch script from a shortcut
    fmt.Println("Switch to window to start capturing...")
    if runtime.GOOS == "linux" {
        time.Sleep(5 * time.Second)
    }

    active_window_name := getActiveWindowName()

    // Extract the bounds from the Window - downside here is that it will
    // always include a toolbar. Also extract window_name for Chrome window
    // for better file naming
    // TODO: provide option to configure this
    // x,y,w,h, active_window_name := window_sizes("Chrome"); _=window_name

    // Option 2: bounds are already defined by user
    x,y := bounds.Min.X, bounds.Min.Y
    w := bounds.Max.X - bounds.Min.X
    h := bounds.Max.Y - bounds.Min.Y

    fmt.Println("Capture geometry", x,y,w,h)

    var img *image.RGBA
    var img_prev *image.RGBA

    // NOTE: requires to run for the first time:
    // sudo chmod +0666 /dev/uinput
    kb, err := keybd_event.NewKeyBonding()
    if err != nil {
        panic(err)
    }


    // Select keys to be pressed to advance to new page
    kb.SetKeys(keybd_event.VK_SPACE)

    // Run until we encounter the same page twice
    // NOTE: to interrupt Ctrl+C
    page := 0
    for  {
        // TODO currently saving to TempDir as there is no good control if
        // screencapture is triggered from as keyboard shortcut
        fileName := fmt.Sprintf("%s/%s_page%04d.png", os.TempDir(), active_window_name, page )
        // fileName := fmt.Sprintf("page%04d.png", page )

        fmt.Printf("Page: %d path: \"%s\"\n", page, fileName)

        img = capture_image(x,y,w,h)

        if same := diff_images(img, img_prev); same == true{
            // Stopping - no cleanup is required as
            // we are stopping before saving next image
            fmt.Println("stopping...")
            break
        }

        // update previous image with the current one
        img_prev = img

        file, _ := os.Create(fileName)
        // TODO: invoking the tool with OS short - needs to set up path correctly
        if err := png.Encode(file, img); err != nil{
            fmt.Printf("error encoding %s\n", err)
        }

        // fmt.Printf("closing file #%d : %v \"%s\"\n", i, bounds, fileName)
        file.Close()
        // Next Page

        kb.Press()
        time.Sleep(10 * time.Millisecond)
        kb.Release()
        // wait some time until the page scrolls - TODO: this may need some tuning
        time.Sleep(1000 * time.Millisecond)

        page += 1
    }

    // scroll to the beginning
    time.Sleep(2 * time.Second)
    kb.HasSHIFT(true)  // now scrolls up with shoft+space

    for i := 0; i < page; i++ {
        kb.Press()
        time.Sleep(10 * time.Millisecond)
        kb.Release()
    }


    // Ouput Pdf
    pdf := gofpdf.New("P", "mm", "A4", ""); _ =pdf

    files := fmt.Sprintf("%s/%s*.png", os.TempDir(), active_window_name )
    paths, _ := filepath.Glob(files)

    for _, path := range paths{
        fmt.Println(path)
        pdf.AddPage()
        pdf.Image(path, 0, 0, 211, 0, false, "", 0, "")
    }
    outpath := fmt.Sprintf("%s/%s.pdf", os.TempDir(), active_window_name)
    fmt.Println(outpath)
    err = pdf.OutputFileAndClose(outpath)
    if err != nil{
        fmt.Println(err)
    }

 }
