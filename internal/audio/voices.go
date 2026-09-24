package audio

// Voice is one entry of ElevenlabsAudioService::VOICES.
type Voice struct {
	Key, ID, Name, Gender string
}

// Voices ports ElevenlabsAudioService::VOICES, in declaration order.
var Voices = []Voice{
	{Key: "male_1", ID: "YNOujSUmHtgN6anjqXPf", Name: "Victor Power", Gender: "male"},
	{Key: "female_1", ID: "lRbfoJL2IRJBT7ma6o7n", Name: "Rita", Gender: "female"},
	{Key: "male_2", ID: "h96v1HCJtcisNNeagp0R", Name: "Will", Gender: "male"},
}
