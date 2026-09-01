package ui

const BrandURL = "https://delbysoft.com"

func Brand(styles Styles) string {
	return styles.BrandDelby.Render("delby") + styles.BrandSoft.Render("soft")
}
