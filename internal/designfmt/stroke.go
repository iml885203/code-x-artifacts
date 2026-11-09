package designfmt

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const (
	maxVectorCoordinate = 1000000
	maxPathBytes        = 65536
	maxPathNumbers      = 32768
)

var (
	svgStrokeTags  = setOf("svg g path circle ellipse rect line polyline polygon text tspan textpath symbol use marker")
	strokeUnits    = setOf("px pt pc in cm mm em rem ex ch vw vh vmin vmax %")
	pathArity      = map[byte]int{'M': 2, 'L': 2, 'H': 1, 'V': 1, 'C': 6, 'S': 4, 'Q': 4, 'T': 2, 'A': 7, 'Z': 0}
	strokeVariable = regexp.MustCompile(`^--[A-Za-z_][A-Za-z0-9_-]{0,79}$`)
	svgNumber      = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?`)
)

// CheckPath accepts SVG's finite path grammar, including compact arc flags, and
// bounds its parsing/rendering cost before a frame or room update is retained.
func CheckPath(path string) error {
	if len(path) > maxPathBytes {
		return fmt.Errorf("a path is at most %d bytes", maxPathBytes)
	}
	p := pathReader{s: path}
	p.space()
	if p.at == len(path) {
		return fmt.Errorf("a path must start with a move")
	}
	first := true
	for p.at < len(p.s) {
		cmd := p.s[p.at]
		p.at++
		upper := strings.ToUpper(string(cmd))[0]
		size, ok := pathArity[upper]
		if !ok || first && upper != 'M' {
			return fmt.Errorf("a path must use SVG commands and start with a move")
		}
		first = false
		p.numberBefore = false
		if size != 0 {
			if err := p.groups(cmd, upper, size); err != nil {
				return err
			}
		}
		p.space()
	}
	return nil
}

func (p *pathReader) groups(cmd, upper byte, size int) error {
	groups := 0
	for {
		p.space()
		if p.at == len(p.s) || strings.ContainsRune("MmLlHhVvCcSsQqTtAaZz", rune(p.s[p.at])) {
			if groups == 0 {
				return fmt.Errorf("command %c needs %d coordinates", cmd, size)
			}
			return nil
		}
		if err := p.group(cmd, upper, size); err != nil {
			return err
		}
		groups++
	}
}

func (p *pathReader) group(cmd, upper byte, size int) error {
	for i := 0; i < size; i++ {
		n, err := p.number(upper == 'A' && (i == 3 || i == 4))
		if err != nil {
			return fmt.Errorf("command %c: %w", cmd, err)
		}
		if upper == 'A' && (i == 0 || i == 1) && n < 0 {
			return fmt.Errorf("arc radii cannot be negative")
		}
	}
	return nil
}

type pathReader struct {
	s            string
	at, numbers  int
	numberBefore bool
}

func (p *pathReader) space() {
	for p.at < len(p.s) && strings.ContainsRune(" \t\r\n", rune(p.s[p.at])) {
		p.at++
	}
}

func (p *pathReader) number(flag bool) (float64, error) {
	p.space()
	if p.at < len(p.s) && p.s[p.at] == ',' {
		if !p.numberBefore {
			return 0, fmt.Errorf("a comma must follow a coordinate")
		}
		p.at++
		p.space()
	}
	if p.at == len(p.s) {
		return 0, fmt.Errorf("missing coordinate")
	}
	var raw string
	if flag {
		if p.s[p.at] != '0' && p.s[p.at] != '1' {
			return 0, fmt.Errorf("an arc flag is 0 or 1")
		}
		raw = p.s[p.at : p.at+1]
	} else {
		raw = svgNumber.FindString(p.s[p.at:])
	}
	if raw == "" {
		return 0, fmt.Errorf("a coordinate must be a finite number")
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || math.Abs(n) > maxVectorCoordinate {
		return 0, fmt.Errorf("a coordinate must be finite and between -%d and %d", maxVectorCoordinate, maxVectorCoordinate)
	}
	p.at += len(raw)
	p.numbers++
	p.numberBefore = true
	if p.numbers > maxPathNumbers {
		return 0, fmt.Errorf("a path has at most %d coordinates", maxPathNumbers)
	}
	return n, nil
}

func checkVectorAttribute(tag, key, value string) error {
	if tag == "path" && key == "d" {
		return CheckPath(value)
	}
	if key == "stroke-width" && svgStrokeTags[tag] {
		return checkStrokeWidth(value)
	}
	return nil
}

func checkStrokeWidth(value string) error {
	v := strings.TrimSpace(value)
	if strings.HasPrefix(v, "var(") && strings.HasSuffix(v, ")") {
		name := strings.TrimSpace(v[4 : len(v)-1])
		if strokeVariable.MatchString(name) {
			return nil
		}
	}
	raw := svgNumber.FindString(v)
	unit := strings.TrimPrefix(v, raw)
	if raw == "" || unit != "" && !strokeUnits[unit] {
		return fmt.Errorf("stroke width must be a finite length or a variable reference")
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > maxVectorCoordinate {
		return fmt.Errorf("stroke width must be finite, nonnegative and at most %d", maxVectorCoordinate)
	}
	return nil
}
