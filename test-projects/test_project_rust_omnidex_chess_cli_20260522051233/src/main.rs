use omnidex_chess_cli::{play_engine_turn, render_board, should_play_again, GameState, OmnidexProvider};
use std::io::{self, Write};

fn main() {
    println!("Omnidex Chess");
    loop {
        let mut state = GameState::default();
        let mut omnidex = OmnidexProvider::default();
        loop {
            println!("{}", render_board(&state.board));
            println!("{}", state.status_text());
            if state.board.status() != chess::BoardStatus::Ongoing {
                break;
            }
            print!("Your move (UCI, help, resign): ");
            io::stdout().flush().expect("flush prompt");
            let mut input = String::new();
            io::stdin().read_line(&mut input).expect("read move");
            let input = input.trim();
            if input.eq_ignore_ascii_case("resign") {
                println!("You resigned. Omnidex wins.");
                break;
            }
            if input.eq_ignore_ascii_case("help") {
                println!("Enter legal moves in UCI notation, for example e2e4. Promotions use e7e8q.");
                continue;
            }
            match state.apply_uci(input) {
                Ok(()) => {}
                Err(err) => {
                    println!("Invalid move: {err}");
                    continue;
                }
            }
            if state.board.status() != chess::BoardStatus::Ongoing {
                println!("{}", state.status_text());
                break;
            }
            match play_engine_turn(&mut state, &mut omnidex) {
                Ok(mv) => println!("Omnidex plays {mv}"),
                Err(err) => {
                    println!("Omnidex failed to provide a legal move: {err}");
                    break;
                }
            }
        }
        print!("Play again? (y/N): ");
        io::stdout().flush().expect("flush play-again prompt");
        let mut again = String::new();
        io::stdin().read_line(&mut again).expect("read play-again answer");
        if !should_play_again(&again) {
            println!("Good game.");
            break;
        }
    }
}
