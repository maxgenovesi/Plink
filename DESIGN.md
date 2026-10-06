# General Game Design
Genre: Top-down 2D arena shooter. Players move with WASD, aim with the mouse, and shoot projectiles.

## Lobbies
Every match is played from a lobby.
- A player creates a lobby and becomes its host. The host picks the match settings and invites others with a shareable link/code.
- Players in the lobby pick a team and mark themselves ready. The match starts once every player is ready and the teams are valid for the chosen settings.
- When a match ends, everyone returns to the same lobby. The host can change settings, invite more players, and start another match.
- If the host leaves, the lobby passes to another player. A lobby is closed once it is empty.

## Match settings (chosen by the host before a match)
- Game mode
- Player count / team size
- Number of rounds
- Map
- Weapons (the "guns" players spawn with)
- Pickups on/off (weapon pickups, throwables)

## Default match
1v1 deathmatch, best of 5 rounds.
- A round ends as soon as one player dies. Both players respawn and the next round starts.
- The first player to win 3 rounds wins the match.

## Content
Maps, game modes, weapons and pickups are data that gets added over time. Adding a new one should not mean rewriting the game loop or the lobby.
- Map: arena size, spawn points per team, obstacles. The first map is a plain ~1500x1500 arena.
- Visuals: circles for players, lines for projectiles.
