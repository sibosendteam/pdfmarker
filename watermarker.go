package pdfmarker

import (
	"errors"
	"gopkg.in/gographics/imagick.v2/imagick"
	"log"
)

var EnableLog = false

type ImageIOSize struct {
	InResolution *Coordinate
	OutScale     float64
}

type Coordinate struct {
	X, Y float64
}

type TextStyle struct {
	Size      *Coordinate
	Font      string
	PointSize float64
	Weight    uint
	Color     string
	Opacity   float64
}

type WatermarkStyle struct {
	Opacity  float64
	Degrees  float64
	Autofit  bool // fit Size of the original image
	position *Coordinate
}

type ImageWatermark struct {
	Style *WatermarkStyle
	File  string
}

func (iwm *ImageWatermark) NewWatermark() (*imagick.MagickWand, error) {
	debugf("Reading watermark from %v", iwm.File)

	mw, err := readFromFile(iwm.File, nil, imagick.ALPHA_CHANNEL_FLATTEN)
	if err != nil {
		return mw, err
	}
	if err := mw.ModulateImage(100, 100, 100); err != nil {
		return mw, err
	}

	//mw.SetBackgroundColor(newTransparentBackgroundPixelWand())
	//mw.SetImageOpacity(.1)
	return mw, nil
}

type TextWatermark struct {
	Style     *WatermarkStyle
	TextStyle *TextStyle
	Text      string
}

func (twm *TextWatermark) NewWatermark() (*imagick.MagickWand, error) {
	mw := imagick.NewMagickWand()
	debug("Initiating watermark")
	// set Text drawing wand params
	dw := imagick.NewDrawingWand()
	if err := dw.SetFont(twm.TextStyle.Font); err != nil {
		return mw, err
	}
	dw.SetFontSize(twm.TextStyle.PointSize)
	dw.SetFontWeight(twm.TextStyle.Weight)
	textPW := imagick.NewPixelWand()
	textPW.SetColor(twm.TextStyle.Color)
	dw.SetFillColor(textPW)
	dw.SetFillOpacity(twm.TextStyle.Opacity)

	debugf("Set Font Style: %v", twm.TextStyle)

	// calc watermark Size by Font metrics
	debug("Calculating metrics")
	//metrics := mw.QueryFontMetrics(dw, twm.Text) // FIXME BUG HERE, memory leak?
	//watermarkRows := uint(metrics.MaximumHorizontalAdvance)
	//watermarkCols := uint(metrics.TextWidth)
	//fmt.Printf("Calculated metrics: %v\n", metrics)

	watermarkCols := uint(twm.TextStyle.Size.X)
	watermarkRows := uint(twm.TextStyle.Size.Y)

	// create watermark image
	watermarkPW := newTransparentBackgroundPixelWand()
	debug("Inited watermark pixel wand")
	if err := mw.NewImage(watermarkCols, watermarkRows, watermarkPW); err != nil {
		return mw, err
	}
	debug("Inited watermark image")

	// draw Text on watermark
	dw.SetGravity(imagick.GRAVITY_SOUTH) // for drawing from 0, 0
	dw.Annotation(0, 0, twm.Text)
	if err := mw.DrawImage(dw); err != nil {
		return mw, err
	}
	debug("Drawed watermark")
	//writeImage(mw, "jpg", "watermark.jpg", true)

	return mw, nil
}

type WatermarkCreator interface {
	NewWatermark() (*imagick.MagickWand, error)
}

func AddWatermark(src string, dst string, inout *ImageIOSize, watermark *imagick.MagickWand, style *WatermarkStyle) (err error) {

	debugf("Adding watermark to %v", src)
	debugf("Reading from %v", src)
	image, err := readFromFile(src, nil, imagick.ALPHA_CHANNEL_FLATTEN)
	defer func() {
		debugf("Destroy image: %v", image)
		if image != nil {
			image.Destroy()
		}
	}()
	if err != nil {
		debug("Reading failed: " + err.Error())
		return err
	}
	//defer image.Destroy() // image is nil if error, therefore not need to destroy
	debugf("Read size %d,%d", image.GetImageWidth(), image.GetImageHeight())
	debug("Iterating images")
	image.ResetIterator()
	if !image.HasNextImage() {
		// TODO do nothing if the input file is not a pdf now.
		//  We need to identify if it is a image and optimize func compositeWatermark
		return errors.New("Not need to add watermark. ")
	}

	debug("Composite watermark")
	if err := setWatermarkStyle(image, watermark, style); err != nil {
		return err
	}
	if err := compositeWatermark(image, watermark, style); err != nil {
		return err
	}

	if err := resize(image, inout.OutScale); err != nil {
		return err
	}

	//if err := writeImage(image, "jpg", dst+".jpg", true); err != nil {
	//	return err
	//}
	debugf("Writing to %v", dst)
	if err := writeImage(image, "pdf", dst, true); err != nil {
		return err
	}
	return nil
}

func setWatermarkStyle(image *imagick.MagickWand, watermark *imagick.MagickWand, style *WatermarkStyle) error {
	// move up
	if style != nil {
		if style.Degrees != 0 {
			debugf(
				"Watermark image Size %v, %v", watermark.GetImageWidth(), watermark.GetImageHeight())
			if err := watermark.RotateImage(newTransparentBackgroundPixelWand(), style.Degrees); err != nil {
				return err
			}
			debugf(
				"Watermark rotated Size %v, %v", watermark.GetImageWidth(), watermark.GetImageHeight())
		}
		if style.Autofit {
			// TODO autofit in height direction to put watermark to the middle of the image
			imageWidth := image.GetImageWidth()
			watermarkWidth := watermark.GetImageWidth()
			scale := float64(imageWidth) / float64(watermarkWidth)
			debugf("Scale: %v", scale)
			if err := resize(watermark, scale); err != nil {
				return err
			}
			debugf(
				"Watermark autofitted Size %v, %v", watermark.GetImageWidth(), watermark.GetImageHeight())
		}
	}
	return nil
}

func compositeWatermark(image *imagick.MagickWand, watermark *imagick.MagickWand, style *WatermarkStyle) error {
	image.ResetIterator()
	debug("Compositing watermark and image: ")
	for image.HasNextImage() {
		image.NextImage()
		var x, y int
		if style.Autofit {
			x = 0
			y = int((image.GetImageHeight() - watermark.GetImageHeight()) / 2)
		} else {
			x = int(style.position.X)
			y = int(style.position.Y)
		}

		if err := image.CompositeImage(watermark, imagick.COMPOSITE_OP_OVER, x, y); err != nil {
			return err
		}
	}
	debug(image.GetIteratorIndex(), "finished")
	return nil
}

func writeImage(mw *imagick.MagickWand, format string, dst string, writeAllImages bool) error {
	if err := mw.SetCompression(imagick.COMPRESSION_JPEG); err != nil {
		return err
	}
	if err := mw.SetCompressionQuality(90); err != nil {
		return err
	}
	if err := mw.SetFormat(format); err != nil {
		return err
	}
	if writeAllImages {
		if err := mw.WriteImages(dst, true); err != nil {
			return err
		}
	} else {
		if err := mw.WriteImage(dst); err != nil {
			return err
		}
	}
	return nil
}

func resize(mw *imagick.MagickWand, scale float64) error {

	// Get original image Size
	width := mw.GetImageWidth()
	height := mw.GetImageHeight()

	// Calculate scaled Size
	sWidth := uint(float64(width) * scale)
	sHeight := uint(float64(height) * scale)

	debugf("Resizing from %v, %v to %v, %v", width, height, sWidth, sHeight)

	// Resize the image using the Lanczos filter
	// The blur factor is a float, where > 1 is blurry, < 1 is sharp
	mw.ResetIterator()
	if mw.HasNextImage() {
		for mw.HasNextImage() {
			mw.NextImage()
			if err := mw.ResizeImage(sWidth, sHeight, imagick.FILTER_LANCZOS, 1); err != nil {
				return err
			}
		}
	} else {
		if err := mw.ResizeImage(sWidth, sHeight, imagick.FILTER_LANCZOS, 1); err != nil {
			return err
		}
	}
	return nil
}

func readFromFile(file string, resolution *Coordinate, alphaChan imagick.AlphaChannelType) (*imagick.MagickWand, error) {

	mw := imagick.NewMagickWand()

	// Must be *before* ReadImageFile
	// Make sure our image is high quality
	if resolution != nil {
		if err := mw.SetResolution(resolution.X, resolution.Y); err != nil {
			return nil, err
		}
	}

	// Load the image File into imagick
	if err := mw.ReadImage(file); err != nil {
		return nil, err
	}

	// Must be *after* ReadImageFile
	// Flatten image and remove alpha channel, to prevent alpha turning black in jpg
	if err := mw.SetImageAlphaChannel(alphaChan); err != nil {
		return nil, err
	}
	return mw, nil
}

func newTransparentBackgroundPixelWand() *imagick.PixelWand {
	pw := imagick.NewPixelWand()
	pw.SetColor("none")
	return pw
}

//func newWhiteBackgroundPixelWand() *imagick.PixelWand {
//	pw := imagick.NewPixelWand()
//	pw.SetColor("white")
//	return pw
//}

func debug(v ...interface{}) {
	if EnableLog {
		log.Println(v...)
	}
}

func debugf(format string, v ...interface{}) {
	if EnableLog {
		log.Printf(format, v...)
	}
}
