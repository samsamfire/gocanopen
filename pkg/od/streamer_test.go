package od

// func TestStreamer(t *testing.T) {
// 	od := createOD()
// 	entry := od.Index(0x3018)
// 	assert.NotNil(t, entry)
// 	// Test access to subindex > 1 for variable
// 	_, err := NewStreamer(entry, 1, true)
// 	assert.Equal(t, ODR_SUB_NOT_EXIST, err)
// 	// Test that subindex 0 returns nil
// 	_, err = NewStreamer(entry, 0, true)
// 	assert.Nil(t, err)
// 	// Test access to subindex 0 of Record should return nil
// 	entry = od.Index(0x3030)
// 	_, err = NewStreamer(entry, 0, true)
// 	assert.Nil(t, err)
// 	// Test access to out of range subindex
// 	_, err = NewStreamer(entry, 10, true)
// 	assert.Equal(t, ODR_SUB_NOT_EXIST, err)

// }
