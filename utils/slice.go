package utils

// UniqueUint removes duplicate values from a slice of uints.
func UniqueUint(slice []uint) []uint {
	keys := make(map[uint]bool)
	list := []uint{}
	for _, entry := range slice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}
