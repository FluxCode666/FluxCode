package chatgptweb

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mrand "math/rand/v2"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/sha3"
)

const DefaultPowScript = "https://chatgpt.com/backend-api/sentinel/sdk.js"

var (
	cores        = []int{8, 16, 24, 32}
	documentKeys = []string{"_reactListeningo743lnnpvdg", "location"}

	navigatorKeys = []string{
		"registerProtocolHandler−function registerProtocolHandler() { [native code] }",
		"storage−[object StorageManager]",
		"locks−[object LockManager]",
		"appCodeName−Mozilla",
		"permissions−[object Permissions]",
		"share−function share() { [native code] }",
		"webdriver−false",
		"managed−[object NavigatorManagedData]",
		"canShare−function canShare() { [native code] }",
		"vendor−Google Inc.",
		"mediaDevices−[object MediaDevices]",
		"vibrate−function vibrate() { [native code] }",
		"storageBuckets−[object StorageBucketManager]",
		"mediaCapabilities−[object MediaCapabilities]",
		"cookieEnabled−true",
		"virtualKeyboard−[object VirtualKeyboard]",
		"product−Gecko",
		"presentation−[object Presentation]",
		"onLine−true",
		"mimeTypes−[object MimeTypeArray]",
		"credentials−[object CredentialsContainer]",
		"serviceWorker−[object ServiceWorkerContainer]",
		"keyboard−[object Keyboard]",
		"gpu−[object GPU]",
		"doNotTrack",
		"serial−[object Serial]",
		"pdfViewerEnabled−true",
		"language−zh-CN",
		"geolocation−[object Geolocation]",
		"userAgentData−[object NavigatorUAData]",
		"getUserMedia−function getUserMedia() { [native code] }",
		"sendBeacon−function sendBeacon() { [native code] }",
		"hardwareConcurrency−32",
		"windowControlsOverlay−[object WindowControlsOverlay]",
	}

	windowKeys = []string{
		"0", "window", "self", "document", "name", "location",
		"customElements", "history", "navigation", "innerWidth",
		"innerHeight", "scrollX", "scrollY", "visualViewport",
		"screenX", "screenY", "outerWidth", "outerHeight",
		"devicePixelRatio", "screen", "chrome", "navigator",
		"onresize", "performance", "crypto", "indexedDB",
		"sessionStorage", "localStorage", "scheduler", "alert",
		"atob", "btoa", "fetch", "matchMedia", "postMessage",
		"queueMicrotask", "requestAnimationFrame", "setInterval",
		"setTimeout", "caches", "__NEXT_DATA__", "__BUILD_MANIFEST",
		"__NEXT_PRELOADREADY",
	}
)

func legacyParseTime() string {
	loc := time.FixedZone("EST", -5*3600)
	now := time.Now().In(loc)
	return now.Format("Mon Jan 02 2006 15:04:05") + " GMT-0500 (Eastern Standard Time)"
}

func BuildPowConfig(userAgent string, scriptSources []string, dataBuild string) []any {
	if len(scriptSources) == 0 {
		scriptSources = []string{DefaultPowScript}
	}
	nowMs := float64(time.Now().UnixMilli())
	return []any{
		[]int{3000, 4000, 5000}[mrand.IntN(3)],
		legacyParseTime(),
		4294705152,
		0,
		userAgent,
		scriptSources[mrand.IntN(len(scriptSources))],
		dataBuild,
		"en-US",
		"en-US,es-US,en,es",
		0,
		navigatorKeys[mrand.IntN(len(navigatorKeys))],
		documentKeys[mrand.IntN(len(documentKeys))],
		windowKeys[mrand.IntN(len(windowKeys))],
		nowMs,
		uuid.New().String(),
		"",
		cores[mrand.IntN(len(cores))],
		nowMs - nowMs,
	}
}

func PowGenerate(seed, difficulty string, config []any, limit int) (string, bool) {
	target, err := hex.DecodeString(difficulty)
	if err != nil {
		return "", false
	}
	diffLen := len(difficulty) / 2
	seedBytes := []byte(seed)

	// Build static JSON segments matching Python's approach:
	// config[:3] -> trim "]", append ","
	// config[4:9] -> trim leading "[" and trailing "]", wrap with ","
	// config[10:] -> trim leading "["
	s1, _ := json.Marshal(config[:3])
	s2, _ := json.Marshal(config[4:9])
	s3, _ := json.Marshal(config[10:])

	// static1 = "[...," (without trailing "]", plus ",")
	static1 := append(s1[:len(s1)-1], ',')
	// static2 = ",...," (strip "[" and "]", wrap with commas)
	static2 := append([]byte{','}, s2[1:len(s2)-1]...)
	static2 = append(static2, ',')
	// static3 = ",...]" (strip "[", prepend ",")
	static3 := append([]byte{','}, s3[1:]...)

	for i := 0; i < limit; i++ {
		iBytes := []byte(fmt.Sprintf("%d", i))
		halfI := []byte(fmt.Sprintf("%d", i>>1))

		finalJSON := make([]byte, 0, len(static1)+len(iBytes)+len(static2)+len(halfI)+len(static3))
		finalJSON = append(finalJSON, static1...)
		finalJSON = append(finalJSON, iBytes...)
		finalJSON = append(finalJSON, static2...)
		finalJSON = append(finalJSON, halfI...)
		finalJSON = append(finalJSON, static3...)

		encoded := base64.StdEncoding.EncodeToString(finalJSON)
		h := sha3.New512()
		h.Write(seedBytes)
		h.Write([]byte(encoded))
		digest := h.Sum(nil)
		if bytes.Compare(digest[:diffLen], target) <= 0 {
			return encoded, true
		}
	}
	fallback := "wQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D" + base64.StdEncoding.EncodeToString([]byte(`"`+seed+`"`))
	return fallback, false
}

func BuildProofToken(seed, difficulty, userAgent string, scriptSources []string, dataBuild string) (string, error) {
	cfg := BuildPowConfig(userAgent, scriptSources, dataBuild)
	answer, solved := PowGenerate(seed, difficulty, cfg, 500000)
	if !solved {
		return "", fmt.Errorf("failed to solve proof token: difficulty=%s", difficulty)
	}
	return "gAAAAAB" + answer, nil
}

func BuildLegacyRequirementsToken(userAgent string, scriptSources []string, dataBuild string) string {
	seed := fmt.Sprintf("%f", mrand.Float64())
	cfg := BuildPowConfig(userAgent, scriptSources, dataBuild)
	answer, _ := PowGenerate(seed, "0fffff", cfg, 500000)
	return "gAAAAAC" + answer
}
