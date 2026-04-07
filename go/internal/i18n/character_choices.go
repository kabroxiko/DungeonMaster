package i18n

// Character choice display labels for locked setup → sheet fields (normalized ids: lowercase, underscores).
// Align with client/dungeonmaster/src/main.js races, classes, subrace_labels, subclass_labels.

type choiceLabel struct {
	en, es string
}

// CharacterChoiceLabel returns a display string for category "race"|"class"|"subrace"|"subclass", or "" if unknown.
func CharacterChoiceLabel(category, normalizedID, locale string) string {
	var tab map[string]choiceLabel
	switch category {
	case "race":
		tab = characterRaceLabels
	case "class":
		tab = characterClassLabels
	case "subrace":
		tab = characterSubraceLabels
	case "subclass":
		tab = characterSubclassLabels
	default:
		return ""
	}
	row, ok := tab[normalizedID]
	if !ok {
		return ""
	}
	if locale == "es" && row.es != "" {
		return row.es
	}
	return row.en
}

var characterRaceLabels = map[string]choiceLabel{
	"dwarf":    {en: "Dwarf", es: "Enano"},
	"elf":      {en: "Elf", es: "Elfo"},
	"gnome":    {en: "Gnome", es: "Gnomo"},
	"half_elf": {en: "Half-Elf", es: "Medio elfo"},
	"half_orc": {en: "Half-Orc", es: "Medio orco"},
	"halfling": {en: "Halfling", es: "Mediano"},
	"human":    {en: "Human", es: "Humano"},
	"tiefling": {en: "Tiefling", es: "Tiflin"},
}

var characterClassLabels = map[string]choiceLabel{
	"artificer": {en: "Artificer", es: "Artífice"},
	"barbarian": {en: "Barbarian", es: "Bárbaro"},
	"bard":      {en: "Bard", es: "Bardo"},
	"cleric":    {en: "Cleric", es: "Clérigo"},
	"druid":     {en: "Druid", es: "Druida"},
	"fighter":   {en: "Fighter", es: "Guerrero"},
	"monk":      {en: "Monk", es: "Monje"},
	"paladin":   {en: "Paladin", es: "Paladín"},
	"ranger":    {en: "Ranger", es: "Explorador"},
	"rogue":     {en: "Rogue", es: "Pícaro"},
	"sorcerer":  {en: "Sorcerer", es: "Hechicero"},
	"warlock":   {en: "Warlock", es: "Brujo"},
	"wizard":    {en: "Wizard", es: "Mago"},
}

var characterSubraceLabels = map[string]choiceLabel{
	"high_elf":       {en: "High Elf", es: "Elfo alto"},
	"wood_elf":       {en: "Wood Elf", es: "Elfo de los bosques"},
	"drow":           {en: "Drow", es: "Drow"},
	"hill_dwarf":     {en: "Hill Dwarf", es: "Enano de las colinas"},
	"mountain_dwarf": {en: "Mountain Dwarf", es: "Enano de la montaña"},
	"lightfoot":      {en: "Lightfoot Halfling", es: "Mediano piesligeros"},
	"stout":          {en: "Stout Halfling", es: "Mediano fornido"},
	"forest_gnome":   {en: "Forest Gnome", es: "Gnomo del bosque"},
	"rock_gnome":     {en: "Rock Gnome", es: "Gnomo de las rocas"},
}

var characterSubclassLabels = map[string]choiceLabel{
	"assassin":         {en: "Assassin", es: "Asesino"},
	"way_of_open_hand": {en: "Way of the Open Hand", es: "Camino del puño abierto"},
	"champion":         {en: "Champion", es: "Campeón"},
	"hunter":           {en: "Hunter", es: "Cazador"},
	"moon":             {en: "Circle of the Moon", es: "Círculo de la luna"},
	"land":             {en: "Circle of the Land", es: "Círculo de la tierra"},
	"lore":             {en: "College of Lore", es: "Colegio del saber"},
	"valor":            {en: "College of Valor", es: "Colegio del valor"},
	"war":              {en: "War Domain", es: "Dominio de la guerra"},
	"life":             {en: "Life Domain", es: "Dominio de la vida"},
	"fiend":            {en: "The Fiend", es: "El maligno"},
	"evocation":        {en: "School of Evocation", es: "Escuela de evocación"},
	"devotion":         {en: "Oath of Devotion", es: "Juramento de devoción"},
	"thief":            {en: "Thief", es: "Ladrón"},
	"draconic":         {en: "Draconic Bloodline", es: "Linaje dracónico"},
	"battle_master":    {en: "Battle Master", es: "Maestro de batalla"},
	"berserker":        {en: "Path of the Berserker", es: "Senda del furioso"},
	"totem":            {en: "Path of the Totem Warrior", es: "Senda del tótem"},
}
