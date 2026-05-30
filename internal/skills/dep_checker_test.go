package skills

import "testing"

func TestImportToPipName(t *testing.T) {
	cases := []struct {
		importName string
		want       string
	}{
		{"cv2", "opencv-python"},
		{"PIL", "Pillow"},
		{"yaml", "pyyaml"},
		{"sklearn", "scikit-learn"},
		{"bs4", "beautifulsoup4"},
		{"dateutil", "python-dateutil"},
		{"dotenv", "python-dotenv"},
		{"pptx", "python-pptx"},
		{"docx", "python-docx"},
		{"attr", "attrs"},
		{"gi", "PyGObject"},
		{"psycopg2", "psycopg2-binary"},
		{"psycopg", "psycopg[binary]"},
		{"MySQLdb", "mysqlclient"},
		{"Crypto", "pycryptodome"},
		{"serial", "pyserial"},
		{"skimage", "scikit-image"},
		{"Levenshtein", "python-Levenshtein"},
		{"requests", "requests"},
		{"numpy", "numpy"},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.importName, func(t *testing.T) {
			if got := importToPipName(tc.importName); got != tc.want {
				t.Errorf("importToPipName(%q) = %q, want %q", tc.importName, got, tc.want)
			}
		})
	}
}

func TestPipToImportName(t *testing.T) {
	cases := []struct {
		pipName string
		want    string
	}{
		{"pillow", "PIL"},
		{"Pillow", "PIL"},
		{"pyyaml", "yaml"},
		{"python-docx", "docx"},
		{"python-pptx", "pptx"},
		{"psycopg2-binary", "psycopg2"},
		{"opencv-python", "cv2"},
		{"plotly", "plotly"},
	}
	for _, tc := range cases {
		t.Run(tc.pipName, func(t *testing.T) {
			if got := pipToImportName(tc.pipName); got != tc.want {
				t.Errorf("pipToImportName(%q) = %q, want %q", tc.pipName, got, tc.want)
			}
		})
	}
}
