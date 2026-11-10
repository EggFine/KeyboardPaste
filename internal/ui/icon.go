package ui

import "fyne.io/fyne/v2"

var appIconSVG = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128">
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0%" stop-color="#2563EB"/>
      <stop offset="100%" stop-color="#1D4ED8"/>
    </linearGradient>
  </defs>
  <rect x="8" y="8" width="112" height="112" rx="26" fill="url(#bg)"/>
  <rect x="20" y="42" width="88" height="58" rx="10" fill="#FFFFFF"/>
  <rect x="28" y="52" width="10" height="10" rx="2" fill="#2563EB"/>
  <rect x="42" y="52" width="10" height="10" rx="2" fill="#2563EB"/>
  <rect x="56" y="52" width="10" height="10" rx="2" fill="#2563EB"/>
  <rect x="70" y="52" width="10" height="10" rx="2" fill="#2563EB"/>
  <rect x="84" y="52" width="16" height="10" rx="2" fill="#1D4ED8"/>
  <rect x="28" y="66" width="14" height="10" rx="2" fill="#2563EB"/>
  <rect x="46" y="66" width="10" height="10" rx="2" fill="#2563EB"/>
  <rect x="60" y="66" width="10" height="10" rx="2" fill="#2563EB"/>
  <rect x="74" y="66" width="26" height="10" rx="2" fill="#1D4ED8"/>
  <rect x="36" y="80" width="56" height="10" rx="3" fill="#2563EB"/>
  <path d="M64 16 L64 36 M55 27 L64 36 L73 27" stroke="#FACC15" stroke-width="6" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
</svg>`)

// AppIcon returns the embedded vector application icon.
func AppIcon() fyne.Resource {
	return fyne.NewStaticResource("keyboardpaste.svg", appIconSVG)
}
