package matrix

import "fmt"

func msgJSON(id, body string) string {
	return fmt.Sprintf(`{"type":"m.room.message","event_id":%q,"sender":"@bob:mock.test","room_id":%q,"origin_server_ts":1,"content":{"msgtype":"m.text","body":%q}}`, id, testRoom, body)
}
