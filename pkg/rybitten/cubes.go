package rybitten

// rgb8 builds an [RGB] from 8-bit channel values. Most cubes are transcribed
// from scanned color wheels in 0–255 form; this keeps the data readable and
// sidesteps Go's integer-division of untyped constants (253/255 would be 0).
func rgb8(r, g, b int) RGB {
	return RGB{float64(r) / 255, float64(g) / 255, float64(b) / 255}
}

// Gamut is a named RYB cube with its provenance — the source palette a [Cube]
// reproduces. Feed a hue wheel through Cube and the result wears this gamut's
// character (see [Palette]).
type Gamut struct {
	Key       string // stable lookup key in Cubes
	Title     string // name of the work or system
	Author    string // who made it
	Year      int    // year of the source
	Reference string // upstream reference image filename, if any
	Cube      Cube
}

// Itten is the default cube: Johannes Itten's chromatic circle (1961), the same
// default RYBitten ships. Corners are, in order, white, red, yellow, orange,
// blue, violet, green, black.
var Itten = Cube{
	rgb8(253, 246, 237), // white (slightly warm)
	rgb8(227, 36, 33),   // red
	rgb8(243, 230, 0),   // yellow
	rgb8(240, 142, 28),  // orange
	rgb8(22, 153, 218),  // blue
	rgb8(120, 34, 170),  // violet
	rgb8(0, 142, 91),    // green
	rgb8(29, 28, 28),    // black
}

// Keys lists every gamut in Cubes in a stable, curated order — historical first
// (oldest theory to modern reference manuals), then contemporary artists, then
// the synthetic CMY/RGB cubes. Range over this for deterministic output; ranging
// a map would scramble the order.
var Keys = []string{
	"itten", "itten-normalized", "itten-neutral",
	"bezold", "boutet", "hett", "schiffermueller", "harris", "harrisc82",
	"harrisc82alt", "goethe", "munsell", "munsell-alt", "hayter", "bormann",
	"albers", "lohse", "church", "chevreul", "runge", "trilobe-synoptique",
	"maycock", "colorprinter", "japschool", "kindergarten1890", "marvel-news",
	"arquitetura-decoracao", "apple90s", "apple80s", "clayton",
	"pixelart", "ippsketch", "ryan", "ten",
	"cmy", "rgb",
}

// Cubes maps each key in [Keys] to its [Gamut]. Transcribed verbatim from
// RYBitten's cubes.ts (MIT, © meodai).
var Cubes = map[string]Gamut{
	"itten": {"itten", "Chromatic Circle", "Johannes Itten", 1961, "farbkreis_extended.png", Itten},

	"itten-normalized": {"itten-normalized", "Chromatic Circle (Paper-white)", "Johannes Itten", 1961,
		"Johannes-Itten-The-chromatic-circle-some-exercises-on-the-contrast-of-pure-colors.webp", Cube{
			rgb8(253, 246, 237), rgb8(247, 45, 41), rgb8(253, 203, 0), rgb8(250, 102, 13),
			rgb8(17, 97, 170), rgb8(101, 57, 138), rgb8(70, 139, 73), rgb8(29, 28, 28),
		}},

	"itten-neutral": {"itten-neutral", "Nathan Gossett & Baoquan Chen", "Johannes Itten", 1961, "itten-ryb.pdf", Cube{
		{1, 1, 1}, {1, 0, 0}, {1, 1, 0}, {1, 0.5, 0},
		{0.163, 0.373, 0.6}, {0.5, 0.0, 0.5}, {0.0, 0.66, 0.2}, {0.2, 0.094, 0.0},
	}},

	"bezold": {"bezold", "Farbentafel", "Wilhelm von Bezold", 1874, "Bezold_Farbentafel_1874.jpg", Cube{
		rgb8(245, 238, 226), rgb8(170, 14, 1), rgb8(224, 178, 0), rgb8(217, 104, 5),
		rgb8(18, 107, 145), rgb8(103, 15, 128), rgb8(88, 133, 30), rgb8(44, 37, 30),
	}},

	"boutet": {"boutet", "Twelve-color color circles", "Claude Boutet", 1708, "Boutet_1708_color_circles.jpg", Cube{
		rgb8(254, 250, 226), rgb8(237, 55, 58), rgb8(255, 233, 111), rgb8(250, 102, 13),
		rgb8(33, 112, 163), rgb8(238, 131, 154), rgb8(59, 155, 83), rgb8(24, 10, 1),
	}},

	"hett": {"hett", "RGV Color Wheel", "J. A. H. Hett", 1908, "RGV_color_wheel_1908.png", Cube{
		rgb8(255, 255, 255), rgb8(218, 105, 104), rgb8(255, 244, 122), rgb8(232, 154, 113),
		rgb8(73, 138, 186), rgb8(97, 96, 178), rgb8(144, 191, 140), rgb8(8, 8, 8),
	}},

	"schiffermueller": {"schiffermueller", "Versuch eines Farbensystems", "Ignaz Schiffermüller", 1772, "020_schiffermueller1.jpg", Cube{
		rgb8(240, 234, 214), rgb8(204, 50, 53), rgb8(253, 222, 20), rgb8(230, 152, 92),
		rgb8(1, 88, 140), rgb8(107, 51, 111), rgb8(51, 138, 92), rgb8(55, 39, 23),
	}},

	"harris": {"harris", "The Natural System of Colours", "Moses Harris", 1766, "Moses_Harris_The_Natural_System_of_Colours.jpg", Cube{
		rgb8(249, 232, 209), rgb8(216, 43, 59), rgb8(231, 175, 2), rgb8(224, 89, 31),
		rgb8(92, 123, 145), rgb8(77, 58, 78), rgb8(107, 129, 53), rgb8(14, 8, 7),
	}},

	"harrisc82": {"harrisc82", "The Natural System of Colours", "Moses Harris / C82", 1766, "harrisc82.png", Cube{
		rgb8(241, 236, 213), rgb8(235, 66, 35), rgb8(253, 236, 1), rgb8(254, 130, 39),
		rgb8(3, 7, 171), rgb8(74, 50, 86), rgb8(55, 131, 74), rgb8(2, 1, 0),
	}},

	"harrisc82alt": {"harrisc82alt", "The Natural System of Colours", "Moses Harris / C82", 1766, "harrisc82alt.png", Cube{
		rgb8(238, 232, 206), rgb8(222, 62, 29), rgb8(247, 225, 7), rgb8(254, 130, 39),
		rgb8(4, 6, 139), rgb8(74, 50, 86), rgb8(56, 131, 75), rgb8(2, 1, 0),
	}},

	"goethe": {"goethe", "Farbenkreis", "Johann Wolfgang von Goethe", 1809,
		"Goethe_Farbenkreis_zur_Symbolisierung_des_menschlichen_Geistes-_und_Seelenlebens_1809.jpg", Cube{
			rgb8(239, 235, 225), rgb8(182, 53, 55), rgb8(253, 203, 0), rgb8(222, 69, 20),
			rgb8(95, 157, 191), rgb8(83, 70, 98), rgb8(58, 90, 66), rgb8(8, 9, 13),
		}},

	"munsell": {"munsell", "Munsell Color System", "Albert Henry Munsell", 1905, "munsell-atlas-11.jpg", Cube{
		rgb8(228, 218, 197), rgb8(181, 65, 60), rgb8(229, 193, 81), rgb8(220, 137, 61),
		rgb8(59, 143, 171), rgb8(121, 97, 134), rgb8(13, 170, 114), rgb8(46, 44, 38),
	}},

	"munsell-alt": {"munsell-alt", "A Grammar of Color", "Cleland, T. M. & Albert Henry Munsell", 1921, "munsell-alt.jpg", Cube{
		rgb8(206, 205, 209), rgb8(181, 38, 54), rgb8(221, 187, 23), rgb8(208, 120, 37),
		rgb8(10, 71, 129), rgb8(101, 36, 66), rgb8(75, 129, 131), rgb8(26, 30, 47),
	}},

	"hayter": {"hayter", "New Practical Treatise on the Three Primitive Colours", "Charles Hayter", 1826, "Color_diagram_Charles_Hayter.jpg", Cube{
		rgb8(237, 213, 177), rgb8(167, 33, 28), rgb8(245, 181, 18), rgb8(204, 93, 46),
		rgb8(71, 122, 141), rgb8(99, 79, 93), rgb8(109, 143, 118), rgb8(44, 44, 37),
	}},

	"bormann": {"bormann", "Gouache tint study for Josef Alber's Preliminary Course", "Heinrich-Siegfried Bormann", 1931, "bormann.png", Cube{
		rgb8(240, 236, 235), rgb8(247, 65, 51), rgb8(243, 187, 6), rgb8(251, 130, 2),
		rgb8(37, 71, 169), rgb8(176, 121, 177), rgb8(2, 117, 111), rgb8(41, 42, 45),
	}},

	"albers": {"albers", "Interaction of Color", "Josef Albers", 1942, "albers-color-harmony.jpg", Cube{
		rgb8(231, 235, 237), rgb8(229, 30, 38), rgb8(255, 198, 12), rgb8(245, 119, 34),
		rgb8(17, 97, 170), rgb8(139, 47, 146), rgb8(1, 167, 98), rgb8(0, 0, 1),
	}},

	"lohse": {"lohse", "Kunsthalle Bern Poster", "Richard Paul Lohse", 1970, "lohse.png", Cube{
		rgb8(236, 237, 241), rgb8(200, 75, 49), rgb8(235, 207, 13), rgb8(228, 168, 21),
		rgb8(39, 108, 176), rgb8(188, 57, 104), rgb8(122, 176, 62), rgb8(4, 4, 4),
	}},

	"church": {"church", "An Elementary Manual for Students", "A.H. Church", 1887, "church.png", Cube{
		rgb8(221, 215, 183), rgb8(142, 42, 37), rgb8(217, 194, 18), rgb8(192, 114, 50),
		rgb8(67, 80, 119), rgb8(83, 51, 88), rgb8(99, 130, 47), rgb8(21, 19, 13),
	}},

	"chevreul": {"chevreul", "Cercle chromatique", "Michel Eugène Chevreul", 1839, "Cercle_chromatique_Chevreul_2.jpg", Cube{
		rgb8(241, 236, 230), rgb8(185, 34, 17), rgb8(231, 200, 52), rgb8(232, 90, 26),
		rgb8(26, 70, 79), rgb8(82, 15, 47), rgb8(67, 111, 33), rgb8(29, 28, 28),
	}},

	"runge": {"runge", "Farbenkugel", "Philipp Otto Runge", 1810, "farbenkugel.png", Cube{
		rgb8(238, 221, 177), rgb8(211, 24, 34), rgb8(248, 211, 36), rgb8(242, 116, 30),
		rgb8(51, 114, 143), rgb8(104, 73, 78), rgb8(90, 127, 42), rgb8(13, 17, 19),
	}},

	"trilobe-synoptique": {"trilobe-synoptique", "Trilobe Synoptique", "Charles Lacouture", 1890, "Charles Lacouture, Trilobe Synoptique.jpeg", Cube{
		rgb8(251, 227, 172), rgb8(227, 16, 7), rgb8(255, 216, 0), rgb8(251, 166, 9),
		rgb8(3, 61, 120), rgb8(139, 35, 67), rgb8(115, 131, 18), rgb8(24, 13, 14),
	}},

	"maycock": {"maycock", "Scale of Normal Colors and their Hues", "Mark M. Maycock", 1895, "maycock.png", Cube{
		rgb8(209, 194, 173), rgb8(159, 36, 31), rgb8(231, 191, 6), rgb8(231, 155, 7),
		rgb8(75, 90, 200), rgb8(121, 100, 188), rgb8(115, 179, 63), rgb8(52, 49, 40),
	}},

	"colorprinter": {"colorprinter", "The Color Printer", "John Earhart", 1892, "colorprinter.png", Cube{
		rgb8(250, 248, 244), rgb8(255, 41, 37), rgb8(251, 223, 47), rgb8(253, 151, 35),
		rgb8(31, 106, 184), rgb8(159, 68, 150), rgb8(80, 180, 122), rgb8(36, 38, 39),
	}},

	"japschool": {"japschool", "Japanese Textbook", "Japanese School", 1930, "japschool.png", Cube{
		rgb8(215, 208, 180), rgb8(202, 0, 17), rgb8(220, 170, 0), rgb8(229, 76, 32),
		rgb8(0, 126, 157), rgb8(137, 37, 79), rgb8(0, 110, 60), rgb8(31, 27, 28),
	}},

	"kindergarten1890": {"kindergarten1890", "Kindergarten Workbook", "Milton Bradley", 1890, "kindergarten1890.jpg", Cube{
		rgb8(236, 231, 213), rgb8(188, 32, 43), rgb8(233, 201, 0), rgb8(197, 72, 30),
		rgb8(50, 42, 115), rgb8(116, 48, 101), rgb8(69, 118, 61), rgb8(56, 44, 42),
	}},

	"marvel-news": {"marvel-news", "64 Color Chart on Newsprint", "Marvel Comics", 1982, "marvel-news.png", Cube{
		rgb8(233, 199, 173), rgb8(214, 76, 127), rgb8(238, 204, 124), rgb8(230, 174, 115),
		rgb8(86, 141, 146), rgb8(118, 83, 97), rgb8(196, 192, 118), rgb8(60, 52, 40),
	}},

	"arquitetura-decoracao": {"arquitetura-decoracao", "Sugestões. Arquitetura Decoração", "Unknown — São Paulo", 1956, "arquitetura-decoracao.png", Cube{
		{0.9765, 0.9647, 0.9255}, {0.9765, 0.4392, 0.4431}, {0.949, 0.9059, 0.4157}, {0.9373, 0.5961, 0.498},
		{0.4431, 0.7098, 0.8}, {0.9098, 0.7961, 0.8}, {0.6275, 0.851, 0.4863}, {0.0863, 0.0745, 0.051},
	}},

	"apple90s": {"apple90s", "Macintosh Reference Manual", "Apple", 1990, "apple90s.png", Cube{
		rgb8(255, 244, 216), rgb8(248, 80, 46), rgb8(255, 213, 44), rgb8(254, 129, 5),
		rgb8(0, 124, 197), rgb8(132, 77, 139), rgb8(120, 160, 66), rgb8(2, 4, 6),
	}},

	"apple80s": {"apple80s", "HyperCard User Manual", "Apple", 1989, "apple80s.png", Cube{
		rgb8(254, 249, 246), rgb8(248, 20, 35), rgb8(237, 199, 8), rgb8(254, 128, 11),
		rgb8(48, 140, 206), rgb8(182, 40, 94), rgb8(135, 187, 26), rgb8(29, 27, 28),
	}},

	"clayton": {"clayton", "Intrinsic Value Plate", "Greg Clayton", 2017, "A260P03_IntrinsicValue1.gif", Cube{
		rgb8(246, 248, 244), rgb8(248, 20, 40), rgb8(255, 198, 8), rgb8(248, 140, 18),
		rgb8(8, 41, 148), rgb8(152, 56, 142), rgb8(8, 156, 49), rgb8(12, 17, 15),
	}},

	"pixelart": {"pixelart", "Pixel Art", "Tofu", 2024, "pixelart.png", Cube{
		rgb8(226, 216, 205), rgb8(224, 43, 39), rgb8(251, 204, 38), rgb8(255, 138, 4),
		rgb8(82, 103, 202), rgb8(199, 112, 253), rgb8(104, 182, 90), rgb8(22, 19, 11),
	}},

	"ippsketch": {"ippsketch", "Imposter Syndrome", "Ippsketch", 2021, "ippsketch.png", Cube{
		rgb8(221, 219, 211), rgb8(196, 82, 69), rgb8(196, 167, 80), rgb8(200, 123, 70),
		rgb8(74, 104, 167), rgb8(94, 89, 161), rgb8(86, 139, 70), rgb8(38, 38, 38),
	}},

	"ryan": {"ryan", "Compositions Palette", "Ryan", 2024, "ryan.png", Cube{
		rgb8(237, 235, 236), rgb8(242, 146, 109), rgb8(245, 234, 143), rgb8(247, 194, 115),
		rgb8(89, 118, 212), rgb8(237, 191, 243), rgb8(153, 201, 113), rgb8(50, 63, 66),
	}},

	"ten": {"ten", "Ten", "Roni Kaufman", 2022, "ten.png", Cube{
		rgb8(255, 251, 230), rgb8(238, 86, 46), rgb8(249, 213, 50), rgb8(252, 132, 4),
		rgb8(43, 103, 175), rgb8(246, 137, 163), rgb8(171, 205, 94), rgb8(5, 5, 5),
	}},

	// CMY subtractive primaries in RGB space. Input axes map C→red slot, M→yellow
	// slot, Y→blue slot.
	"cmy": {"cmy", "CMY Subtractive Primaries", "Jacob Christoph Le Blon", 1725, "", Cube{
		{1, 1, 1}, {0, 1, 1}, {1, 1, 0}, {0, 1, 0},
		{1, 0, 1}, {0, 0, 1}, {1, 0, 0}, {0, 0, 0},
	}},

	"rgb": {"rgb", "Inverted RGB", "James Clerk Maxwell", 1860, "rgb-cube.png", Cube{
		{1, 1, 1}, {1, 0, 0}, {0, 1, 0}, {1, 1, 0},
		{0, 0, 1}, {1, 0, 1}, {0, 1, 1}, {0, 0, 0},
	}},
}
