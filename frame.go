package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"time"
)

type Bitmap [128 * 64]bool

func (b *Bitmap) Set(x, y int) {
	if x >= 0 && x < 128 && y >= 0 && y < 64 {
		b[y*128+x] = true
	}
}

var glyphs = map[rune]string{
	'0': "01110/10001/10011/10101/11001/10001/01110", '1': "00100/01100/00100/00100/00100/00100/01110",
	'2': "01110/10001/00001/00010/00100/01000/11111", '3': "11110/00001/00001/01110/00001/00001/11110",
	'4': "00010/00110/01010/10010/11111/00010/00010", '5': "11111/10000/10000/11110/00001/00001/11110",
	'6': "01110/10000/10000/11110/10001/10001/01110", '7': "11111/00001/00010/00100/01000/01000/01000",
	'8': "01110/10001/10001/01110/10001/10001/01110", '9': "01110/10001/10001/01111/00001/00001/01110",
	'A': "01110/10001/10001/11111/10001/10001/10001", 'B': "11110/10001/10001/11110/10001/10001/11110",
	'C': "01111/10000/10000/10000/10000/10000/01111", 'D': "11110/10001/10001/10001/10001/10001/11110",
	'E': "11111/10000/10000/11110/10000/10000/11111", 'F': "11111/10000/10000/11110/10000/10000/10000",
	'G': "01111/10000/10000/10111/10001/10001/01111", 'H': "10001/10001/10001/11111/10001/10001/10001",
	'I': "01110/00100/00100/00100/00100/00100/01110", 'J': "00111/00010/00010/00010/10010/10010/01100",
	'K': "10001/10010/10100/11000/10100/10010/10001", 'L': "10000/10000/10000/10000/10000/10000/11111",
	'M': "10001/11011/10101/10101/10001/10001/10001", 'N': "10001/11001/10101/10011/10001/10001/10001",
	'O': "01110/10001/10001/10001/10001/10001/01110", 'P': "11110/10001/10001/11110/10000/10000/10000",
	'Q': "01110/10001/10001/10001/10101/10010/01101", 'R': "11110/10001/10001/11110/10100/10010/10001",
	'S': "01111/10000/10000/01110/00001/00001/11110", 'T': "11111/00100/00100/00100/00100/00100/00100",
	'U': "10001/10001/10001/10001/10001/10001/01110", 'V': "10001/10001/10001/10001/10001/01010/00100",
	'W': "10001/10001/10001/10101/10101/10101/01010", 'X': "10001/10001/01010/00100/01010/10001/10001",
	'Y': "10001/10001/01010/00100/00100/00100/00100", 'Z': "11111/00001/00010/00100/01000/10000/11111",
	':': "00000/00100/00100/00000/00100/00100/00000", '-': "00000/00000/00000/11111/00000/00000/00000",
	'.': "00000/00000/00000/00000/00000/00110/00110", '%': "11001/11010/00010/00100/01000/01011/10011",
	'/': "00001/00010/00010/00100/01000/01000/10000", ' ': "00000/00000/00000/00000/00000/00000/00000",
}

func (b *Bitmap) Text(s string, x, y, scale int) {
	for i, c := range strings.ToUpper(s) {
		for dy, row := range strings.Split(glyphs[c], "/") {
			for dx, v := range row {
				if v == '1' {
					for sy := 0; sy < scale; sy++ {
						for sx := 0; sx < scale; sx++ {
							b.Set(x+i*6*scale+dx*scale+sx, y+dy*scale+sy)
						}
					}
				}
			}
		}
	}
}
func (b Bitmap) Reports() [][1024]byte {
	out := make([][1024]byte, 2)
	for half := 0; half < 2; half++ {
		r := &out[half]
		r[0], r[1], r[2], r[4], r[5] = 6, 0x93, byte(half*64), 64, 64
		for x := 0; x < 64; x++ {
			for y := 0; y < 64; y++ {
				if b[y*128+half*64+x] {
					bit := x*64 + y
					r[6+bit/8] |= 1 << uint(bit%8)
				}
			}
		}
	}
	return out
}
func number(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "--"
	}
	if v > 999 {
		v = 999
	}
	return fmt.Sprintf("%.0f", v)
}
func render(now time.Time, game string, p Perf, m Metrics) Bitmap {
	var b Bitmap
	minute := now.Unix() / 60
	if game == "" {
		if now.Hour() >= 21 || now.Hour() < 7 {
			if now.Minute()%5 == 0 && now.Second() < 10 {
				drawCat(&b, int((now.Unix()/300*37)%99), int((now.Unix()/300*19)%48))
			}
			return b
		}
		x := int((minute * 17) % 42)
		y := int((minute * 23) % 44)
		b.Text(now.Format("15:04"), x, y, 3)
		return b
	}
	ox, oy := int(minute%3), int((minute/3)%3)
	{
		fps := "--"
		if p.Fresh {
			fps = number(p.FPS)
		}
		b.Text(fps, ox+1, oy+1, 2)
		b.Text("FPS", ox+49, oy+1, 1)
		b.Text("1% LOW", ox+79, oy+1, 1)
		low := "--"
		if p.Fresh && p.Samples >= 100 {
			low = number(p.Low)
		}
		b.Text(low, ox+79, oy+10, 1)
	}
	b.Text("CPU "+number(m.CPU)+"%", ox+1, oy+22, 1)
	b.Text(number(m.CPUTemp)+"C", ox+57, oy+22, 1)
	b.Text("GPU "+number(m.GPU)+"%", ox+1, oy+31, 1)
	b.Text(number(m.GPUTemp)+"C", ox+57, oy+31, 1)
	if p.LatencyHeld {
		b.Text("LAST", ox+96, oy+22, 1)
	} else {
		b.Text("LAT", ox+99, oy+22, 1)
	}
	latency := number(p.Latency) + "MS"
	b.Text(latency, ox+107-(len(latency)*6-1)/2, oy+31, 1)
	b.Text("33", ox, oy+40, 1)
	b.Text("MS", ox, oy+48, 1)
	b.Text("0", ox+5, oy+56, 1)
	previousX, previousY := -1, 0
	for x, v := range graphColumns(p.Graph, 110) {
		if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			previousX = -1
			continue
		}
		px := ox + 16 + x
		py := oy + 61 - int(math.Round(math.Min(v, 33)/33*21))
		if previousX >= 0 {
			b.Line(previousX, previousY, px, py)
		} else {
			b.Set(px, py)
		}
		previousX, previousY = px, py
	}
	return b
}

// Line draws an unfilled, one-pixel-wide connected trace (Bresenham).
func (b *Bitmap) Line(x0, y0, x1, y1 int) {
	dx := int(math.Abs(float64(x1 - x0)))
	dy := -int(math.Abs(float64(y1 - y0)))
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		b.Set(x0, y0)
		if x0 == x1 && y0 == y1 {
			return
		}
		twice := 2 * err
		if twice >= dy {
			err += dy
			x0 += sx
		}
		if twice <= dx {
			err += dx
			y0 += sy
		}
	}
}
func graphColumns(input [116]float64, width int) []float64 {
	out := make([]float64, width)
	for i, v := range input {
		x := i * width / len(input)
		if v > out[x] {
			out[x] = v
		}
	}
	return out
}

var cat = []string{
	"                         #####", "                            # ", "                           #  ", "                          #   ", "                         #####", "                              ",
	"   #     #                    ", "  # #   # #      #######       ", "  #  ###  #   ###       ###    ", " #         ###            #   ", " #  ## ##  #               #  ", " #    #    #                # ", "  #       #      #####      # ", "   #######     ##     #     # ", "    #         #       #    #  ", "     ##        #######   ##   ", "       ##################     ",
}

func drawCat(b *Bitmap, x, y int) {
	for dy, row := range cat {
		for dx, v := range row {
			if v == '#' {
				b.Set(x+dx, y+dy)
			}
		}
	}
}
func savePreview(path string, b Bitmap) error {
	img := image.NewGray(image.Rect(0, 0, 128*6, 64*6))
	for y := 0; y < 64; y++ {
		for x := 0; x < 128; x++ {
			if b[y*128+x] {
				for dy := 0; dy < 6; dy++ {
					for dx := 0; dx < 6; dx++ {
						img.SetGray(x*6+dx, y*6+dy, color.Gray{Y: 255})
					}
				}
			}
		}
	}
	f, e := os.Create(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return png.Encode(f, img)
}
