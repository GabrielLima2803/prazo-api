package domain

import "testing"

func TestCourseCode(t *testing.T) {
	cases := map[string]string{
		"2026/2_CCT_CCI251-03A_POO0002": "POO",
		"2026/2_CCT_CCI251-03A_EDA1002": "EDA",
		"2026/2_CCT_CCI251-03A_AMS0003": "AMS",
		"ALP0001-2026-2":                "ALP",
		"alp":                           "ALP",
		"SITE":                          "SITE",
		"12345":                         "12345",
		"":                              "",
	}
	for in, want := range cases {
		if got := CourseCode(in); got != want {
			t.Errorf("CourseCode(%q) = %q; quer %q", in, got, want)
		}
	}
}
