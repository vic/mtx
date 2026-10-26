# Hidden: start mtx on the seeded server and open #mtx. The recording starts in the room.
Hide
Type "set -a; . docs/tapes/.demo/env; set +a; MTX_DB=docs/tapes/.demo/mtx.db ./mtx"
Enter
Wait+Screen@15s /#mtx/
Type ":theme base24/dracula"
Enter
Sleep 1500ms
Ctrl+O
Sleep 1500ms
Type "mtx"
Sleep 1500ms
Enter
Sleep 2s
Show
