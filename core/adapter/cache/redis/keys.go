package redis

func currentSessionKey(userID string) string {
	return "myai:current_session:" + userID
}
