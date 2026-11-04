package odt

// hostileExtras is every word LibreOffice reads beyond the model's: the three list labels it renders and the placeholder
// line a picture-less figure becomes. Pinned word for word (wtest.ReadBack).
var hostileExtras = []string{"1.", "◦", "2.", "[Picture:", "Scanned", "page", "3", "\"quoted\"", "<b>]"}
