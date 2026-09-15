# Welcome to
# __________         __    __  .__                               __
# \______   \_____ _/  |__/  |_|  |   ____   ______ ____ _____  |  | __ ____
#  |    |  _/\__  \\   __\   __\  | _/ __ \ /  ___//    \\__  \ |  |/ // __ \
#  |    |   \ / __ \|  |  |  | |  |_\  ___/ \___ \|   |  \/ __ \|    <\  ___/
#  |________/(______/__|  |__| |____/\_____>______>___|__(______/__|__\\_____>
#
# This file can be a nice home for your Battlesnake logic and helper functions.
#
# To get you started we've included code to prevent your Battlesnake from moving backwards.
# For more info see docs.battlesnake.com

import random
import typing
from collections import deque

DIRECTIONS = ((0, 1), (0, -1), (-1, 0), (1, 0))


def _neighbors(cell, width, height):
    x, y = cell
    for dx, dy in DIRECTIONS:
        nx, ny = x + dx, y + dy
        if 0 <= nx < width and 0 <= ny < height:
            yield (nx, ny)


def edge_distance(cell, width, height):
    """How many cells a point is from the nearest wall. 0 means touching it."""
    x, y = cell
    return min(x, width - 1 - x, y, height - 1 - y)


def flood_fill_size(start, blocked, width, height):
    """Count cells reachable from start through open space, via BFS."""
    if start in blocked:
        return 0
    visited = {start}
    queue = deque([start])
    while queue:
        cell = queue.popleft()
        for neighbor in _neighbors(cell, width, height):
            if neighbor in blocked or neighbor in visited:
                continue
            visited.add(neighbor)
            queue.append(neighbor)
    return len(visited)


def compute_territory(sources, blocked, width, height):
    """
    Multi-source BFS board-control map. `sources` is {owner: start_cell}.
    Returns {cell: owner} for every reachable cell; a cell reached by two
    owners in the same number of steps is contested and maps to None.
    """
    best_dist = {}
    claimant = {}
    tied = set()
    queue = deque()

    for owner, cell in sources.items():
        if cell in blocked or cell in best_dist:
            continue
        best_dist[cell] = 0
        claimant[cell] = owner
        queue.append(cell)

    while queue:
        cell = queue.popleft()
        dist = best_dist[cell]
        owner = claimant[cell]
        for neighbor in _neighbors(cell, width, height):
            if neighbor in blocked:
                continue
            next_dist = dist + 1
            if neighbor not in best_dist:
                best_dist[neighbor] = next_dist
                claimant[neighbor] = owner
                queue.append(neighbor)
            elif best_dist[neighbor] == next_dist and claimant[neighbor] != owner:
                tied.add(neighbor)

    return {cell: (None if cell in tied else owner) for cell, owner in claimant.items()}


# info is called when you create your Battlesnake on play.battlesnake.com
# and controls your Battlesnake's appearance
# TIP: If you open your Battlesnake URL in a browser you should see this data
def info() -> typing.Dict:
    print("INFO")

    return {
        "apiversion": "1",
        "author": "Gale",
        "color": "#4F9153",  # Smaragdine
        "head": "default",  # TODO: Choose head
        "tail": "default",  # TODO: Choose tail
    }


# start is called when your Battlesnake begins a game
def start(game_state: typing.Dict):
    print("GAME START")


# end is called when your Battlesnake finishes a game
def end(game_state: typing.Dict):
    print("GAME OVER\n")


# move is called on every turn and returns your next move
# Valid moves are "up", "down", "left", or "right"
# See https://docs.battlesnake.com/api/example-move for available data
def move(game_state: typing.Dict) -> typing.Dict:

    is_move_safe = {"up": True, "down": True, "left": True, "right": True}
    move_deltas = {"up": (0, 1), "down": (0, -1), "left": (-1, 0), "right": (1, 0)}

    # We've included code to prevent your Battlesnake from moving backwards
    my_head = game_state["you"]["body"][0]  # Coordinates of your head
    my_neck = game_state["you"]["body"][1]  # Coordinates of your "neck"

    if my_neck["x"] < my_head["x"]:  # Neck is left of head, don't move left
        is_move_safe["left"] = False

    elif my_neck["x"] > my_head["x"]:  # Neck is right of head, don't move right
        is_move_safe["right"] = False

    elif my_neck["y"] < my_head["y"]:  # Neck is below head, don't move down
        is_move_safe["down"] = False

    elif my_neck["y"] > my_head["y"]:  # Neck is above head, don't move up
        is_move_safe["up"] = False

    # Step 1 - Prevent your Battlesnake from moving out of bounds
    board_width = game_state["board"]["width"]
    board_height = game_state["board"]["height"]

    if my_head["x"] == 0:
        is_move_safe["left"] = False
    if my_head["x"] == board_width - 1:
        is_move_safe["right"] = False
    if my_head["y"] == 0:
        is_move_safe["down"] = False
    if my_head["y"] == board_height - 1:
        is_move_safe["up"] = False

    # Step 2 - Prevent your Battlesnake from colliding with itself
    my_body = game_state["you"]["body"]

    if {"x": my_head["x"] - 1, "y": my_head["y"]} in my_body:
        is_move_safe["left"] = False
    if {"x": my_head["x"] + 1, "y": my_head["y"]} in my_body:
        is_move_safe["right"] = False
    if {"x": my_head["x"], "y": my_head["y"] - 1} in my_body:
        is_move_safe["down"] = False
    if {"x": my_head["x"], "y": my_head["y"] + 1} in my_body:
        is_move_safe["up"] = False

    # Step 3 - Prevent your Battlesnake from colliding with other Battlesnakes
    opponents = game_state["board"]["snakes"]

    for snake in opponents:
        if snake["id"] == game_state["you"]["id"]:
            continue
        if {"x": my_head["x"] - 1, "y": my_head["y"]} in snake["body"]:
            is_move_safe["left"] = False
        if {"x": my_head["x"] + 1, "y": my_head["y"]} in snake["body"]:
            is_move_safe["right"] = False
        if {"x": my_head["x"], "y": my_head["y"] - 1} in snake["body"]:
            is_move_safe["down"] = False
        if {"x": my_head["x"], "y": my_head["y"] + 1} in snake["body"]:
            is_move_safe["up"] = False

    # Avoid moves that risk a head-to-head collision with an equal-or-longer snake
    my_length = len(my_body)
    for snake in opponents:
        if snake["id"] == game_state["you"]["id"]:
            continue
        if len(snake["body"]) < my_length:
            continue  # we would win a head-on collision, no need to avoid it

        opp_head = snake["body"][0]
        opp_next_heads = {
            (opp_head["x"] + dx, opp_head["y"] + dy) for dx, dy in move_deltas.values()
        }

        for move, dxdy in move_deltas.items():
            if not is_move_safe[move]:
                continue
            dx, dy = dxdy
            if (my_head["x"] + dx, my_head["y"] + dy) in opp_next_heads:
                is_move_safe[move] = False

    # Are there any safe moves left?
    safe_moves = []
    for move, isSafe in is_move_safe.items():
        if isSafe:
            safe_moves.append(move)

    if len(safe_moves) == 0:
        print(f"MOVE {game_state['turn']}: No safe moves detected! Moving down")
        return {"move": "down"}

    def cell_after(move):
        dx, dy = move_deltas[move]
        return (my_head["x"] + dx, my_head["y"] + dy)

    blocked_cells = {
        (segment["x"], segment["y"]) for snake in opponents for segment in snake["body"]
    }

    # Step 6 - Avoid moves that would trap us in a pocket too small for our own body
    roomy_moves = [
        m
        for m in safe_moves
        if flood_fill_size(cell_after(m), blocked_cells, board_width, board_height) >= my_length
    ]
    candidate_moves = roomy_moves if roomy_moves else safe_moves

    def distance_to_after(target, move):
        dx, dy = move_deltas[move]
        new_x, new_y = my_head["x"] + dx, my_head["y"] + dy
        return abs(target["x"] - new_x) + abs(target["y"] - new_y)

    def move_towards(target, moves):
        best_distance = min(distance_to_after(target, m) for m in moves)
        best_moves = [m for m in moves if distance_to_after(target, m) == best_distance]
        return random.choice(best_moves)

    def territory_score(move, rival_snakes):
        # Rivals already hugging a wall have less room to escape into, so shrinking
        # their space further is weighted as more valuable than doing the same to a
        # rival out in the open middle of the board.
        sources = {"me": cell_after(move)}
        rival_weight = {}
        for i, snake in enumerate(rival_snakes):
            head = (snake["body"][0]["x"], snake["body"][0]["y"])
            rival_id = f"rival{i}"
            sources[rival_id] = head
            rival_weight[rival_id] = 1.0 / (edge_distance(head, board_width, board_height) + 1)

        owners = compute_territory(sources, blocked_cells, board_width, board_height)
        my_area = sum(1 for o in owners.values() if o == "me")
        rival_area = sum(rival_weight[o] for o in owners.values() if o in rival_weight)
        return my_area - rival_area

    def move_claiming_most_territory(moves, rival_snakes):
        scores = {m: territory_score(m, rival_snakes) for m in moves}
        best_score = max(scores.values())
        return random.choice([m for m in moves if scores[m] == best_score])

    other_snakes = [s for s in opponents if s["id"] != game_state["you"]["id"]]
    killable = [s for s in other_snakes if len(s["body"]) < my_length]
    food = game_state["board"]["food"]
    my_health = game_state["you"]["health"]
    LOW_HEALTH = 25  # below this, survival trumps positioning

    def closest_food_target():
        return min(
            food,
            key=lambda f: abs(f["x"] - my_head["x"]) + abs(f["y"] - my_head["y"]),
        )

    if food and my_health <= LOW_HEALTH:
        # Step 4 - Running low on health: food is the priority over cutting anyone off
        next_move = move_towards(closest_food_target(), candidate_moves)
    elif killable:
        # Step 5 - A shorter snake is within reach: squeeze its space to force a
        # head-to-head collision it can't win. Prefer prey already near a wall or
        # corner over merely-closer prey out in the open - they're easier to trap.
        def prey_priority(snake):
            head = (snake["body"][0]["x"], snake["body"][0]["y"])
            distance = abs(head[0] - my_head["x"]) + abs(head[1] - my_head["y"])
            return distance * (edge_distance(head, board_width, board_height) + 1)

        nearest_prey = min(killable, key=prey_priority)
        next_move = move_claiming_most_territory(candidate_moves, [nearest_prey])
    elif other_snakes:
        # Step 7 (Tron mode) - claiming board space is the top priority whenever
        # rivals are alive; food only breaks ties between equally good positions
        scores = {m: territory_score(m, other_snakes) for m in candidate_moves}
        best_score = max(scores.values())
        best_moves = [m for m in candidate_moves if scores[m] == best_score]
        if food and len(best_moves) > 1:
            next_move = move_towards(closest_food_target(), best_moves)
        else:
            next_move = random.choice(best_moves)
    elif food:
        # Step 4 - No opponents on the board: just go get food
        next_move = move_towards(closest_food_target(), candidate_moves)
    else:
        next_move = random.choice(candidate_moves)

    print(f"MOVE {game_state['turn']}: {next_move}")
    return {"move": next_move}


# Start server when `python main.py` is run
if __name__ == "__main__":
    from server import run_server

    run_server({"info": info, "start": start, "move": move, "end": end})
