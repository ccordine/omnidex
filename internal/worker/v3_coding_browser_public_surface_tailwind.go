package worker

import (
	"fmt"
	"strings"
)

var directCodingBrowserSafeTailwindVariants = map[string]struct{}{
	"sm": {}, "md": {}, "lg": {}, "xl": {}, "2xl": {},
	"hover": {}, "focus": {}, "focus-visible": {}, "active": {},
	"disabled": {}, "first": {}, "last": {}, "odd": {}, "even": {},
}

var directCodingBrowserSafeTailwindExact = map[string]struct{}{
	"block": {}, "inline-block": {}, "inline": {}, "flex": {}, "inline-flex": {},
	"grid": {}, "inline-grid": {}, "flow-root": {},
	"table": {}, "inline-table": {}, "table-caption": {}, "table-cell": {},
	"table-footer-group": {},
	"table-header-group": {}, "table-row-group": {}, "table-row": {},
	"visible": {}, "isolate": {}, "isolation-auto": {},
	"box-border": {}, "box-content": {}, "border-collapse": {}, "border-separate": {},
	"flex-row": {}, "flex-row-reverse": {}, "flex-col": {}, "flex-col-reverse": {},
	"flex-wrap": {}, "flex-wrap-reverse": {}, "flex-nowrap": {},
	"grow": {}, "grow-0": {}, "shrink": {}, "shrink-0": {},
	"basis-auto": {}, "basis-full": {},
	"items-start": {}, "items-end": {}, "items-center": {}, "items-baseline": {},
	"items-stretch": {}, "justify-normal": {}, "justify-start": {},
	"justify-end": {}, "justify-center": {}, "justify-between": {},
	"justify-around": {}, "justify-evenly": {}, "justify-stretch": {},
	"content-normal": {}, "content-center": {}, "content-start": {},
	"content-end": {}, "content-between": {}, "content-around": {},
	"content-evenly": {}, "content-baseline": {}, "content-stretch": {},
	"self-auto": {}, "self-start": {}, "self-end": {}, "self-center": {},
	"self-stretch": {}, "self-baseline": {}, "place-content-center": {},
	"place-content-start": {}, "place-content-end": {}, "place-content-between": {},
	"place-content-around": {}, "place-content-evenly": {}, "place-content-baseline": {},
	"place-content-stretch": {}, "place-items-start": {}, "place-items-end": {},
	"place-items-center": {}, "place-items-baseline": {}, "place-items-stretch": {},
	"place-self-auto": {}, "place-self-start": {}, "place-self-end": {},
	"place-self-center": {}, "place-self-stretch": {},
	"w-auto": {}, "w-full": {}, "w-screen": {}, "w-min": {}, "w-max": {}, "w-fit": {},
	"h-auto": {}, "h-full": {}, "h-screen": {}, "h-min": {}, "h-max": {}, "h-fit": {},
	"min-w-full": {}, "min-w-min": {}, "min-w-max": {}, "min-w-fit": {},
	"min-h-full": {}, "min-h-screen": {}, "min-h-min": {}, "min-h-max": {}, "min-h-fit": {},
	"max-w-none": {}, "max-w-xs": {}, "max-w-sm": {}, "max-w-md": {},
	"max-w-lg": {}, "max-w-xl": {}, "max-w-2xl": {}, "max-w-3xl": {},
	"max-w-4xl": {}, "max-w-5xl": {}, "max-w-6xl": {}, "max-w-7xl": {},
	"max-w-full": {}, "max-w-min": {}, "max-w-max": {}, "max-w-fit": {}, "max-w-prose": {},
	"max-h-none": {}, "max-h-full": {}, "max-h-screen": {},
	"max-h-min": {}, "max-h-max": {}, "max-h-fit": {},
	"aspect-auto": {}, "aspect-square": {}, "aspect-video": {},
	"font-sans": {}, "font-serif": {}, "font-mono": {}, "font-thin": {},
	"font-extralight": {}, "font-light": {}, "font-normal": {}, "font-medium": {},
	"font-semibold": {}, "font-bold": {}, "font-extrabold": {}, "font-black": {},
	"italic": {}, "not-italic": {}, "antialiased": {}, "subpixel-antialiased": {},
	"text-left": {}, "text-center": {}, "text-right": {}, "text-justify": {},
	"text-start": {}, "text-end": {}, "text-xs": {}, "text-sm": {}, "text-base": {},
	"text-lg": {}, "text-xl": {}, "text-2xl": {}, "text-3xl": {}, "text-4xl": {},
	"text-5xl": {}, "text-6xl": {}, "text-7xl": {}, "text-8xl": {}, "text-9xl": {},
	"uppercase": {}, "lowercase": {}, "capitalize": {}, "normal-case": {},
	"underline": {}, "overline": {}, "line-through": {}, "no-underline": {},
	"whitespace-normal": {}, "whitespace-pre": {}, "whitespace-pre-line": {},
	"break-normal": {}, "break-words": {}, "break-all": {}, "break-keep": {},
	"hyphens-none": {}, "hyphens-manual": {}, "hyphens-auto": {},
	"list-inside": {}, "list-outside": {}, "list-none": {}, "list-disc": {}, "list-decimal": {},
	"border": {}, "border-0": {}, "border-2": {}, "border-4": {}, "border-8": {},
	"border-solid": {}, "border-dashed": {}, "border-dotted": {},
	"border-double": {}, "border-none": {},
	"shadow": {}, "shadow-2xs": {}, "shadow-xs": {}, "shadow-sm": {},
	"shadow-md": {}, "shadow-lg": {}, "shadow-xl": {}, "shadow-2xl": {}, "shadow-none": {},
	"cursor-auto": {}, "cursor-default": {}, "cursor-pointer": {}, "cursor-text": {},
	"select-auto": {}, "select-text": {}, "select-all": {},
}

var directCodingBrowserSafeTailwindLeading = map[string]struct{}{
	"none": {}, "tight": {}, "snug": {}, "normal": {}, "relaxed": {}, "loose": {},
	"3": {}, "4": {}, "5": {}, "6": {}, "7": {}, "8": {}, "9": {}, "10": {},
}

var directCodingBrowserSafeTailwindTracking = map[string]struct{}{
	"tighter": {}, "tight": {}, "normal": {}, "wide": {}, "wider": {}, "widest": {},
}

var directCodingBrowserSafeTailwindSpacing = map[string]struct{}{
	"0": {}, "px": {}, "0.5": {}, "1": {}, "1.5": {}, "2": {}, "2.5": {},
	"3": {}, "3.5": {}, "4": {}, "5": {}, "6": {}, "7": {}, "8": {},
	"9": {}, "10": {}, "11": {}, "12": {}, "14": {}, "16": {}, "20": {},
	"24": {}, "28": {}, "32": {}, "36": {}, "40": {}, "44": {}, "48": {},
	"52": {}, "56": {}, "60": {}, "64": {}, "72": {}, "80": {}, "96": {},
}

func validateDirectCodingBrowserSafeTailwindClass(class string) error {
	if class == "" || strings.ContainsAny(class, "[]!/") {
		return fmt.Errorf("browser public surface rejects non-allowlisted Tailwind class %q", class)
	}
	parts := strings.Split(class, ":")
	base := parts[len(parts)-1]
	for _, variant := range parts[:len(parts)-1] {
		if _, safe := directCodingBrowserSafeTailwindVariants[variant]; !safe {
			return fmt.Errorf("browser public surface rejects non-allowlisted Tailwind class %q", class)
		}
	}
	if strings.HasPrefix(base, "-") || !directCodingBrowserSafeTailwindBase(base) {
		return fmt.Errorf("browser public surface rejects non-allowlisted Tailwind class %q", class)
	}
	return nil
}

func directCodingBrowserSafeTailwindBase(base string) bool {
	if _, safe := directCodingBrowserSafeTailwindExact[base]; safe {
		return true
	}
	if directCodingBrowserSafeSpacingClass(base) ||
		directCodingBrowserSafeGridClass(base) ||
		directCodingBrowserSafeTypographyClass(base) ||
		directCodingBrowserSafeBorderClass(base) ||
		directCodingBrowserSafeRadiusClass(base) {
		return true
	}
	return base == "overflow-visible" || base == "overflow-x-visible" ||
		base == "overflow-y-visible"
}

func directCodingBrowserSafeSpacingClass(base string) bool {
	if base == "mx-auto" {
		return true
	}
	for _, prefix := range []string{
		"p-", "px-", "py-", "ps-", "pe-", "pt-", "pr-", "pb-", "pl-",
		"gap-", "gap-x-", "gap-y-",
	} {
		if suffix, found := strings.CutPrefix(base, prefix); found {
			_, safe := directCodingBrowserSafeTailwindSpacing[suffix]
			return safe
		}
	}
	return false
}

func directCodingBrowserSafeGridClass(base string) bool {
	for _, prefix := range []string{"grid-cols-", "grid-rows-"} {
		if suffix, found := strings.CutPrefix(base, prefix); found {
			return directCodingBrowserSmallPositiveInteger(suffix, 12)
		}
	}
	for _, prefix := range []string{"col-span-", "row-span-"} {
		if suffix, found := strings.CutPrefix(base, prefix); found {
			return directCodingBrowserSmallPositiveInteger(suffix, 12) || suffix == "full"
		}
	}
	for _, prefix := range []string{"col-start-", "col-end-", "row-start-", "row-end-"} {
		if suffix, found := strings.CutPrefix(base, prefix); found {
			return directCodingBrowserSmallPositiveInteger(suffix, 13) || suffix == "auto"
		}
	}
	return false
}

func directCodingBrowserSmallPositiveInteger(value string, maximum int) bool {
	if value == "" || len(value) > 2 {
		return false
	}
	number := 0
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
		number = number*10 + int(character-'0')
	}
	return number >= 1 && number <= maximum
}

func directCodingBrowserSafeTypographyClass(base string) bool {
	if suffix, found := strings.CutPrefix(base, "leading-"); found {
		_, safe := directCodingBrowserSafeTailwindLeading[suffix]
		return safe
	}
	if suffix, found := strings.CutPrefix(base, "tracking-"); found {
		_, safe := directCodingBrowserSafeTailwindTracking[suffix]
		return safe
	}
	return false
}

func directCodingBrowserSafeBorderClass(base string) bool {
	for _, side := range []string{"x", "y", "s", "e", "t", "r", "b", "l"} {
		prefix := "border-" + side
		if base == prefix {
			return true
		}
		for _, width := range []string{"0", "2", "4", "8"} {
			if base == prefix+"-"+width {
				return true
			}
		}
	}
	return false
}

func directCodingBrowserSafeRadiusClass(base string) bool {
	if base == "rounded" {
		return true
	}
	for _, shape := range []string{"s", "e", "t", "r", "b", "l", "ss", "se", "ee", "es", "tl", "tr", "br", "bl"} {
		if base == "rounded-"+shape {
			return true
		}
	}
	for _, shape := range []string{"", "s-", "e-", "t-", "r-", "b-", "l-", "ss-", "se-", "ee-", "es-", "tl-", "tr-", "br-", "bl-"} {
		for _, size := range []string{"none", "xs", "sm", "md", "lg", "xl", "2xl", "3xl", "4xl", "full"} {
			if base == "rounded-"+shape+size {
				return true
			}
		}
	}
	return false
}
