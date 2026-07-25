# GoTicTacToe terminal game

This is a terminal multiplayer TicTacToe game written in Go.

### Key requirements
- Meaningful use of architecture and explained stack decisions
- End-to-end game workflow is correct(correct moves tracking, human-readable errors for wrong moves, etc.)
- Tests that cover game core and auth logic
- User uses auth protocol thoughout the game

#### Some additional requirements
- Matchmaking logic - a queue based system that automatically pairs users
- Persistence - store games in DB
- Deployment - Dockerfile or a docker-compose.yaml
- Real-tine - WS instead of polling for updates
- Leaderboard - user could easily track his stats

### Stack used
//TODO

### Starting a project
```shell
    go run ./cmd/
```
//TODO

### Project structute
- internal - _main game logic_
- config - _config files for different external services and dockerfiles_
- cmd - _entry points used for starting client or server_ 

### Architecture decisiond
//TODO
