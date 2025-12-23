package ai

import "strings"

// Categories that should be removed from tags
var blockedCategories = map[string]bool{
	"politics":     true,
	"economy":      true,
	"business":     true,
	"technology":   true,
	"world":        true,
	"siyaset":      true, // Turkish equivalents
	"ekonomi":      true,
	"iş dünyası":   true,
	"teknoloji":    true,
	"dünya":        true,
}

// organizationVariations maps different variations of organization names to their canonical form
var organizationVariations = map[string]string{
	// AK Party variations
	"akp":                      "AK Party",
	"ak partisi":               "AK Party",
	"akparti":                  "AK Party",
	"ak parti":                 "AK Party",
	"akp hükümeti":             "AK Party",
	"adalet ve kalkınma partisi": "AK Party",
	"justice and development party": "AK Party",

	// Republican People's Party variations
	"republican people's party":    "CHP",
	"cumhuriyet halk partisi":      "CHP",
	"cumhuriyet halk partisi (chp)": "CHP",
	"chp":                          "CHP",
	"Republican People's Party (CHP)":                          "CHP",

	// Grand National Assembly of Türkiye variations
	"grand national assembly of türkiye": "TBMM",
	"turkish grand national assembly":    "TBMM",
	"turkish grand national assembly (tbmm)": "TBMM",
	"türkiye büyük millet meclisi":       "TBMM",
	"tbmm":                               "TBMM",

	// DEM Party variations
	"dem parti": "DEM Party",

	// Constitutional Court variations
	"anayasa mahkemesi": "Constitutional Court",

	// Turkish Statistical Institute variations
	"turkish statistical institute":  "TUIK",
	"türkiye statistical institute":  "TUIK",
	"tuik":                           "TUIK",

	// IYI Party variations
	"iyi parti": "IYI Party",

	// Turkey Labour Party variations
	"türkiye işçİ partisi": "TİP",
	"türkiye işçi partisi": "TİP",
	"turkey işci partisi": "TİP",
	"tip": "TİP",

	// Kamuyu Aydınlatma Platformu variations
	"kamuyu aydınlatma platformu": "KAP",
	"kap": "KAP",

	// Nationalist Movement Party variations
	"nationalist movement party": "MHP",
	"mhp": "MHP",

	// Federal Bureau of Investigation variations
	"federal bureau of investigation": "FBI",
	"fbi": "FBI",

	// Central Bank of the Republic of Türkiye variations
	"central bank of the republic of türkiye": "TCMB",
	"tcmb": "TCMB",

	// Workers' Party of Türkiye variations
	"workers' party of türkiye": "TİP",

	// Great Union Party variations
	"büyük birlik partisi": "BBP",
	"bbp": "BBP",

	// Nuclear Regulatory Authority variations
	"nükleer düzenleme kurumu": "NDK",
	"nuclear regulatory authority": "NDK",
	"ndk": "NDK",

	// Energy Market Regulatory Authority variations
	"energy market regulatory authority": "EPDK",
	"enerji piyasası düzenleme kurumu": "EPDK",
	"epdk": "EPDK",

	// European Union variations
	"european union": "EU",
	"eu": "EU",

	// Organisation for Economic Co-operation and Development variations
	"organisation for economic co-operation and development": "OECD",
	"oecd": "OECD",

	// YPG/PKK variations
	"ypg/pkk": "YPG",
	"ypg": "YPG",

	// Supreme Election Council variations
	"supreme election council": "YSK",
	"supreme election council (ysk)": "YSK",
	"yuksek secim kurulu": "YSK",
	"yüksek seçim kurulu": "YSK",
	"ysk": "YSK",

	// European Court of Human Rights variations
	"european court of human rights": "ECHR",
	"echr": "ECHR",

	// High Council of Judges and Prosecutors variations
	"high council of judges and prosecutors": "HSYK",
	"high council of judges and prosecutors (hsk)": "HSYK",
	"hsyk": "HSYK",

	// Financial Crimes Investigation Board variations
	"financial crimes investigation board": "MASAK",
	"financial crimes investigation board (masak)": "MASAK",
	"masak": "MASAK",

	// Add more organization normalization rules here as needed
}

// ShouldRemoveTag checks if a tag should be removed based on blocked categories
func ShouldRemoveTag(tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	return blockedCategories[tag]
}

// NormalizeOrganization normalizes organization names to their canonical form
func NormalizeOrganization(org string) string {
	if org == "" {
		return org
	}

	// Try to find a normalized version (case-insensitive)
	lowerOrg := strings.ToLower(org)
	if normalized, exists := organizationVariations[lowerOrg]; exists {
		return normalized
	}

	// Check for partial matches (e.g., if the organization name contains a variation)
	for variation, normalized := range organizationVariations {
		if strings.Contains(lowerOrg, variation) {
			return normalized
		}
	}

	// Return original if no normalization rule matches
	return org
}
