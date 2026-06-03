package slp

type Player struct {
	Port        int    `json:"port"`
	DisplayName string `json:"displayName,omitempty"`
	ConnectCode string `json:"connectCode,omitempty"`
	SlippiUID   string `json:"slippiUid,omitempty"`
}

type Match struct {
	MatchID          string   `json:"matchId"`
	SessionID        string   `json:"sessionId"`
	GameNumber       uint32   `json:"gameNumber"`
	TiebreakerNumber uint32   `json:"tiebreakerNumber"`
	PlayerCodes      []string `json:"playerCodes"`
	Players          []Player `json:"players"`
	GameEnded        bool     `json:"gameEnded"`
}
