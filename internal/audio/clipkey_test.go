package audio

import (
	"os"
	"testing"
)

// Keys computed by the Rails oracle (Audio::ClipKey.for) with no TTS
// environment overrides.
func TestClipKeyMatchesRails(t *testing.T) {
	for _, env := range []string{"AUDIO_TTS_PROVIDER", "OPENAI_TTS_MODEL", "OPENAI_TTS_VOICE", "OPENAI_TTS_SPEED",
		"OPENAI_TTS_INSTRUCTIONS", "GOOGLE_TTS_MODEL", "GOOGLE_TTS_VOICE", "GOOGLE_TTS_SPEED", "GOOGLE_TTS_INSTRUCTIONS"} {
		if v, ok := os.LookupEnv(env); ok {
			os.Unsetenv(env)
			t.Cleanup(func() { os.Setenv(env, v) })
		}
	}
	pt, en := "pt-BR", "en"
	cases := []struct {
		provider, want string
	}{
		{ClipKey(openAI(&pt), "Amém."), "606406e2-d44b-5b23-86e1-d0c39aa59d11"},
		{ClipKey(google(&en), "Amen."), "d4e809a1-5231-53f4-8cf9-20653cae7215"},
	}
	for _, c := range cases {
		if c.provider != c.want {
			t.Errorf("got %s want %s", c.provider, c.want)
		}
	}
}
