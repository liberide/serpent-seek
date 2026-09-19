package providers

import "encoding/xml"

// xmlText collects character data of an element including any nested markup
// (e.g. Yandex <hlword> highlights, PubMed <AbstractText> formatting tags).
type xmlText struct {
	Text string
}

// UnmarshalXML concatenates all character data inside the element.
func (t *xmlText) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch tt := tok.(type) {
		case xml.CharData:
			t.Text += string(tt)
		case xml.EndElement:
			if tt.Name == start.Name {
				return nil
			}
		}
	}
}
