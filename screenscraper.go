// A tool to automate screen-capturing long web pages and turn them into a Pdf.
// Usage:
// Run a tool, shift-select a region to capture
// The script takes a screen-shot and advance to the next fragment of the page.
// Once the end of the page is reached, all captured images are converted to the Pdf
// NOTE:
// Linux support only at the moment as it exclusively rely on xcb (X11) package
package main

 import (
    "image/png"
    "image"
	"image/color"
    "os"
    "fmt"
    "log"
    "time"
	"strings"
	"flag"

    "github.com/BurntSushi/xgbutil"
    "github.com/BurntSushi/xgbutil/ewmh"
    "github.com/BurntSushi/xgbutil/xwindow"
	"github.com/BurntSushi/xgb/xtest"

    "github.com/BurntSushi/xgbutil/mousebind"
    "github.com/BurntSushi/xgbutil/xevent"

    "github.com/BurntSushi/xgb/xproto"

    "github.com/BurntSushi/xgbutil/xgraphics"  // painting
    "github.com/BurntSushi/xgb"  // active window


    // "github.com/micmonay/keybd_event"
    "runtime"
    "regexp"

    "github.com/jung-kurt/gofpdf"
    "path/filepath"
)


var undos [][]byte  // undos are stored here
var WIDTH = 1       // Width of the brushe used for drawing bounding box

// This function returns the name of the current active window
// Currently it is only used to programaticly generate name from resulted screenshots
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
		}
	}
	return destination_window
}

func bringWindowAbove(X *xgbutil.XUtil, destination_window xproto.Window){
	// NOTE: Using a workaround.
	// Instead of just ewmh.ActiveWindowSet which has no effect.
	// We set Focus which then receives key events
	xproto.SetInputFocus(X.Conn(), xproto.InputFocusParent, destination_window, xproto.TimeCurrentTime)

	fmt.Println("active window is now", destination_window)

	ewmh.WmStateReq(X, destination_window, ewmh.StateToggle, "_NET_WM_STATE_ABOVE")

	// Looks like all we need after changing active windows is to give some time
	// for X server to refresh. This will assure the first captured image
	// will not store previous window order
	time.Sleep(1*time.Second)

}

func disableWindowAbove(X *xgbutil.XUtil, destination_window xproto.Window){
	ewmh.WmStateReq(X, destination_window, ewmh.StateRemove, "_NET_WM_STATE_ABOVE")
}

func nextPage(X *xgbutil.XUtil, destination_window xproto.Window) {
	// https://www.x.org/releases/X11R7.7/doc/xextproto/xtest.html
	PAGE_DOWN := 117

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

func drawRestorePrevious(canvas *xgraphics.Image, win *xwindow.Window, x, y, prev_x, prev_y int) {

	///// back to the previously modified image

	img := undos[len(undos)-1]
	_ = img
	counter := 0
	for i := 0; i < len(canvas.Pix); i += 4 {
		_i := i / 4
		_x := _i % (canvas.Stride / 4)
		_y := _i / (canvas.Stride / 4)

		_, _ = _x,_y

		b := img[i]
		g := img[i+1]
		r := img[i+2]
		_, _,_ = b,g,r

		// Reset previous row or collumns

		// Direction - right and down
		/*
		start 
		     \
		      \
		       end
		*/
		if (_x >= prev_x-WIDTH && _x < x) || (_y >= prev_y-WIDTH && _y < y) {
			canvas.Set(_x, _y, color.RGBA{r, g, b, 255})
		}

		// Direction - left and up
		/*
		end
		   \
		    \
		     start
		*/
		if (_x <= prev_x+WIDTH && _x > x) || (_y <= prev_y+WIDTH && _y > y) {
			canvas.Set(_x, _y, color.RGBA{r, g, b, 255})
		}		
	}
	canvas.XDraw()
	canvas.XPaint(win.Id)
	fmt.Println("counter", counter)
}

func drawRect(canvas *xgraphics.Image, win *xwindow.Window, x, y, start_x, start_y, prev_x, prev_y int) {

	// restore original image (this avoids ghosting)
	drawRestorePrevious(canvas, win, x, y, prev_x, prev_y)

	rectXtop := image.Rect(start_x, start_y, x, start_y-WIDTH)
	rectXbottom := image.Rect(start_x, y, x, y-WIDTH)
	rectYleft := image.Rect(start_x, start_y, start_x-WIDTH, y)
	rectYright := image.Rect(x, start_y, x-WIDTH, y)

	bounds_arr := []image.Rectangle{rectXtop, rectXbottom, rectYleft, rectYright}
	pencil := xgraphics.BGRA{0x00, 0xff, 0x0, 125}

	for _, rect := range bounds_arr {

		if rect.Empty() {
			continue
		}
		// Create the subimage of the canvas to draw to.
		tip := canvas.SubImage(rect).(*xgraphics.Image)

		// Now color each pixel in tip with the pencil color.
		tip.For(func(x, y int) xgraphics.BGRA {
			return xgraphics.BlendBGRA(tip.At(x, y).(xgraphics.BGRA), pencil /* color*/)
		})

		// Now draw the changes to the pixmap.
		tip.XDraw()

		// And paint them to the window.
		tip.XPaint(win.Id)
	}
}

// Function to allow user input and select area of the screen to capture
// Currenlty the bounding box is represented by paint stroke where
// start (mouse-down) represents top-left corner
// release (mouse-up) represents bottom-right corner
// Release of the mouse button closes the X window and let program continue
// but reuquires to switch active window to the one we want to capture.
func getCaptureArea() (rect image.Rectangle) {

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

    win := canvas.XShowExtra("Select area to capture", true)

    // Once initialized turn to fullscreen (f11) to match coordinates on screen
    ewmh.WmStateReq(canvas.X, win.Id, ewmh.StateToggle, "_NET_WM_STATE_FULLSCREEN")
       err = mousebind.ButtonPressFun(
        func(X *xgbutil.XUtil, e xevent.ButtonPressEvent) {
            log.Println("A second handler always happens after the first.")
        }).Connect(X, X.RootWin(), "1", false, true)

	// before first brush stroke - push original image onto undo stack
	undo_step := make([]byte, len(canvas.Pix))
	copy(undo_step, canvas.Pix)
	undos = append(undos, undo_step)
	
	// cropping
	var start_rx, start_ry, prev_x, prev_y int
    mousebind.Drag(X, X.RootWin(), X.RootWin(), "1", false,
        func(X *xgbutil.XUtil, rx, ry, ex, ey int) (bool, xproto.Cursor) {
            log.Println("starting", rx, ry)
            bounds.Min.X = rx
            bounds.Min.Y = ry

			start_rx = rx
			start_ry = ry
			prev_x = rx
			prev_y = ry
            return true, 0
        },
        func(X *xgbutil.XUtil, rx, ry, ex, ey int) {
            log.Println("pressed", rx, ry)			
            drawRect(canvas, win , rx, ry, start_rx, start_ry, prev_x, prev_y)
			prev_x = rx
			prev_y = ry

        },
        func(X *xgbutil.XUtil, rx, ry, ex, ey int) {
            log.Println("release", rx, ry)
            bounds.Max.X = rx
            bounds.Max.Y = ry

            // graceful exit
            xevent.Detach(win.X, win.Id)
            mousebind.Detach(win.X, win.Id)
            win.Destroy()
            xevent.Quit(X)
        })

    if err != nil {
        log.Fatal(err)
    }

    // Record the area to process
    xevent.Main(X)

    // Continue with other stuff
    fmt.Println("Bounds to process", bounds)

    return bounds
}

func capture_image(X *xgbutil.XUtil, x,y,w,h int) (*image.RGBA){
    var img *image.RGBA

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
			//A := canvas.Pix[index+3]  // on Arm64 we get 0 instead of 255
			img.Set(ix,jy, color.RGBA{B,G,R,255})
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

func test_capture(X *xgbutil.XUtil){
	// img := capture_image(0,64,34, 20)
	img := capture_image(X,0,0,500,500)
	if img == nil{
		fmt.Println("returning no screen capture taken")
		return
	}
	file, _ := os.Create("/tmp/aaa.png")
	// TODO: invoking the tool with OS short - needs to set up path correctly
	if err := png.Encode(file, img); err != nil{
		fmt.Printf("error encoding %s\n", err)
	}
}
type linept struct{
	x,y int
}
func test_draw_line(){
	// (76,153)-(280,203)
	pts := []linept{{76,153}, {76,155}, {77,166}, {78,184}, {82,206}, {89,228}, {100,247}, {115,261}, {134,269}, {160,269}, {191,260}, {222,247}, {248,231}, {265,220}, {275,213}, {280,208}, {283,206}, {284,204}, {284,204}, {284,203}, {284,203}, {283,203}, {283,202}, {283,202}, {283,202}, {283,202}, {282,202}, {281,203}, {280,203}, {280,203}}
	fmt.Println(pts)
	_ = pts



}

func main() {
	var windowFlag = flag.String("w", "Chrom", "Name of the window to capture. The default window name is 'Chrom' which will Chrome and Chromium on some platforms.")
	var totalPagesFlag = flag.Int("p", -1, "Total numer of pages to capture. The default -1, does not interupt capturing.")
	flag.Parse()

	// test_draw_line()
	// return

    bounds := getCaptureArea()

	// Give some delay
    if runtime.GOOS == "linux" {
        time.Sleep(1 * time.Second)
    }

    // Option 2: bounds are already defined by user
    x,y := bounds.Min.X, bounds.Min.Y
    w := bounds.Max.X - bounds.Min.X
    h := bounds.Max.Y - bounds.Min.Y

    fmt.Println("Capture geometry", x,y,w,h)

    var img *image.RGBA
    var img_prev *image.RGBA

    X, err := xgbutil.NewConn()
    if err != nil {
        log.Fatal(err)
    }

	xtest.Init(X.Conn())

	destination_window := getWindowId(X, *windowFlag)
	//TODO exit if not found
	bringWindowAbove(X, destination_window)

	// TODO - reuse destination_window
	active_window_name := getActiveWindowName()
	
    // Run until we encounter the same page twice
    // NOTE: to interrupt Ctrl+C
    page := 0
    for  {
        // TODO currently saving to TempDir as there is no good control if
        // screencapture is triggered from as keyboard shortcut
        fileName := fmt.Sprintf("%s/%s_page%04d.png", os.TempDir(), active_window_name, page )
        // fileName := fmt.Sprintf("page%04d.png", page )

        fmt.Printf("Page: %d path: \"%s\"\n", page, fileName)

        img = capture_image(X,x,y,w,h)

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
		nextPage(X, destination_window)
		
        // wait some time until the page scrolls - NOTE: this may need some tuning in the future
        time.Sleep(1000 * time.Millisecond)

        page += 1

		if *totalPagesFlag > 0 && page >= *totalPagesFlag{
			break
		}
    }


	// Once done remove this property
	defer disableWindowAbove(X, destination_window)

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
