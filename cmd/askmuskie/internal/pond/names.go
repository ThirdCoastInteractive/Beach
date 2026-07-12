package pond

import (
	"fmt"
	"hash/fnv"
	"math"
)

// randomName and colorFor derive a stable visitor identity from the session id,
// so a given visitor keeps the same name and marker color for their visit
// without any accounts. Both are deterministic in sid.

var nameAdjectives = []string{
	"Quiet", "Old", "Lucky", "Restless", "Wise", "Hungry", "Sly", "Calm",
	"Bold", "Patient", "Drifting", "Murky", "Silver", "Mossy", "Lonesome",
	"Curious", "Lazy", "Brave", "Humble", "Cunning",
}

var nameNouns = []string{
	"Pike", "Carp", "Perch", "Minnow", "Bass", "Trout", "Walleye", "Sunfish",
	"Eel", "Crawdad", "Heron", "Otter", "Reed", "Ripple", "Lily", "Skipper",
	"Angler", "Bobber", "Current", "Shoal",
}

func hash32(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

// randomName builds an "Adjective Noun" handle from sid.
func randomName(sid string) string {
	h := hash32(sid)
	adj := nameAdjectives[h%uint32(len(nameAdjectives))]
	noun := nameNouns[(h/uint32(len(nameAdjectives)))%uint32(len(nameNouns))]
	return adj + " " + noun
}

// colorFor returns a vivid #rrggbb marker color, hue chosen from sid.
func colorFor(sid string) string {
	hue := float64(hash32(sid+"color")%360) / 360.0
	r, g, b := hslToRGB(hue, 0.65, 0.6)
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

func hslToRGB(h, s, l float64) (uint8, uint8, uint8) {
	var r, g, b float64
	if s == 0 {
		r, g, b = l, l, l
	} else {
		q := l + s - l*s
		if l < 0.5 {
			q = l * (1 + s)
		}
		p := 2*l - q
		r = hue2rgb(p, q, h+1.0/3.0)
		g = hue2rgb(p, q, h)
		b = hue2rgb(p, q, h-1.0/3.0)
	}
	return uint8(math.Round(r * 255)), uint8(math.Round(g * 255)), uint8(math.Round(b * 255))
}

func hue2rgb(p, q, t float64) float64 {
	if t < 0 {
		t += 1
	}
	if t > 1 {
		t -= 1
	}
	switch {
	case t < 1.0/6.0:
		return p + (q-p)*6*t
	case t < 1.0/2.0:
		return q
	case t < 2.0/3.0:
		return p + (q-p)*(2.0/3.0-t)*6
	default:
		return p
	}
}
