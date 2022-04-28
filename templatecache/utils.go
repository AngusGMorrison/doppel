package templatecache

// or returns the first signal from one or more channels of the same type.
func or[T any](channels ...<-chan T) <-chan T {
	switch len(channels) {
	case 0:
		return nil
	case 1:
		return channels[0]
	}

	orChan := make(chan T)

	go func() {
		// When we receive a signal on any channel, the goroutine will return,
		// closing orChan. goroutines listening to orChan then start receiving
		// its zero-value. This communicates that a signal has been received on
		// at least one channel.
		defer close(orChan)

		switch len(channels) {
		case 2:
			// Every recursive call has at least 2 channels. Having a special
			// case for exactly 2 channels helps limit the depth of the
			// recursion.
			select {
			case <-channels[0]:
			case <-channels[1]:
			}
		default:
			// At this point we know that the slice contains at least 3
			// channels.
			select {
			case <-channels[0]:
			case <-channels[1]:
			case <-channels[2]:
			// Recursively create an or-channel from the remaining channels in
			// the slice, forming a tree that returns when the first signal is
			// received. Passing the orChan of the current node into the
			// recursive function ensures that a signal received by node up the
			// tree will also cause those lower in the tree to return.
			case <-or(append(channels[3:], orChan)...):
			}
		}
	}()

	return orChan
}
