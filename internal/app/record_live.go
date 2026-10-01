package app

func (c cli) recordLive(args []string) int {
	return c.runLive(args, "record-live")
}
