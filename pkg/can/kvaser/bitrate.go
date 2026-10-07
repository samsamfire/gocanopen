package kvaser

import "fmt"

// Predefined canlib bitrates (canBITRATE_* in canlib.h)
const (
	Bitrate1M   = -1
	Bitrate500k = -2
	Bitrate250k = -3
	Bitrate125k = -4
	Bitrate100k = -5
	Bitrate62k  = -6
	Bitrate50k  = -7
	Bitrate83k  = -8
	Bitrate10k  = -9
)

// canlibBitrate maps a bitrate in bit/s to its predefined canlib constant.
func canlibBitrate(bitrate int) (int32, error) {
	switch bitrate {
	case 1_000_000:
		return Bitrate1M, nil
	case 500_000:
		return Bitrate500k, nil
	case 250_000:
		return Bitrate250k, nil
	case 125_000:
		return Bitrate125k, nil
	case 100_000:
		return Bitrate100k, nil
	case 62_500:
		return Bitrate62k, nil
	case 50_000:
		return Bitrate50k, nil
	case 83_333:
		return Bitrate83k, nil
	case 10_000:
		return Bitrate10k, nil
	default:
		return 0, fmt.Errorf("kvaser: unsupported bitrate %v", bitrate)
	}
}
