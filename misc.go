package canopen

func isIDRestricted(canId uint16) bool {
	return canId <= 0x7f ||
		(canId >= 0x101 && canId <= 0x180) ||
		(canId >= 0x581 && canId <= 0x5FF) ||
		(canId >= 0x601 && canId <= 0x67F) ||
		(canId >= 0x6E0 && canId <= 0x6FF) ||
		canId >= 0x701
}
