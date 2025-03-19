package main

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFFT(t *testing.T) {

	a := []complex128{complex(5, 0), complex(3, 0), complex(7, 0)}
	b := []complex128{complex(7, 0), complex(2, 0), complex(1, 0)}
	assert.Equal(t, []int{35, 31, 60, 17, 7}, PolyMulti(a, b), "fft error")
}

func PolyMulti(a []complex128, b []complex128) []int {
	al := len(a)
	bl := len(b)
	max_a_mul_b := al + bl
	var n2 = 1
	for n2 < max_a_mul_b {
		n2 <<= 1
	}
	for len(a) < n2 {
		a = append(a, complex(0, 0))
	}
	for len(b) < n2 {
		b = append(b, complex(0, 0))
	}
	FFT(a, n2, 1)
	FFT(b, n2, 1)
	for i := 0; i < n2; i++ {
		a[i] = a[i] * b[i]
	}
	FFT(a, n2, -1)
	var res = make([]int, 0, n2)
	for i := 0; i < n2; i++ {
		v := int(real(a[i]) / float64(n2))
		if v <= 0 {
			break
		}
		res = append(res, v)
	}
	return res
}

func FFT(poly []complex128, n int, op int) {
	if n == 1 {
		return
	}
	var poly1, poly2 = make([]complex128, n/2), make([]complex128, n/2)
	for i := 0; i < n/2; i++ {
		poly1[i] = poly[i*2]
		poly2[i] = poly[i*2+1]
	}
	FFT(poly1, n/2, op)
	FFT(poly2, n/2, op)
	w1 := complex(math.Cos(float64(2.0*math.Pi)/float64(n)), math.Sin(float64(2.0*math.Pi)/float64(n)*float64(op)))
	wk := complex(1, 0)
	for i := 0; i < n/2; i++ {
		poly[i] = poly1[i] + poly2[i]*wk
		poly[i+n/2] = poly1[i] - poly2[i]*wk
		wk = wk * w1
	}
}
