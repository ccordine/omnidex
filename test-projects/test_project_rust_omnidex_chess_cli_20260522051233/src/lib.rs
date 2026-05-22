use chess::{Board, BoardStatus, ChessMove, Color, File, MoveGen, Piece, Rank, Square};
use std::process::{Command, Stdio};

pub trait MoveProvider {
    fn choose_move(&mut self, state: &GameState) -> Result<String, String>;
}

#[derive(Clone)]
pub struct GameState {
    pub board: Board,
}

impl Default for GameState {
    fn default() -> Self {
        Self { board: Board::default() }
    }
}

impl GameState {
    pub fn legal_moves(&self) -> Vec<ChessMove> {
        MoveGen::new_legal(&self.board).collect()
    }

    pub fn legal_uci_moves(&self) -> Vec<String> {
        self.legal_moves().into_iter().map(|m| m.to_string()).collect()
    }

    pub fn apply_uci(&mut self, mv: &str) -> Result<(), String> {
        let parsed = parse_uci_move(mv)?;
        if !MoveGen::new_legal(&self.board).any(|legal| legal == parsed) {
            return Err(format!("illegal move: {mv}"));
        }
        self.board = self.board.make_move_new(parsed);
        Ok(())
    }

    pub fn status_text(&self) -> &'static str {
        match self.board.status() {
            BoardStatus::Ongoing => "Game in progress",
            BoardStatus::Stalemate => "Draw by stalemate",
            BoardStatus::Checkmate => {
                if self.board.side_to_move() == Color::White {
                    "Black wins by checkmate"
                } else {
                    "White wins by checkmate"
                }
            }
        }
    }
}

pub fn parse_uci_move(input: &str) -> Result<ChessMove, String> {
    let clean = input.trim().to_lowercase();
    let bytes = clean.as_bytes();
    if bytes.len() != 4 && bytes.len() != 5 {
        return Err("moves must use UCI notation like e2e4 or e7e8q".into());
    }
    let from = square(&clean[0..2])?;
    let to = square(&clean[2..4])?;
    let promotion = if bytes.len() == 5 {
        match bytes[4] as char {
            'q' => Some(Piece::Queen),
            'r' => Some(Piece::Rook),
            'b' => Some(Piece::Bishop),
            'n' => Some(Piece::Knight),
            _ => return Err("promotion must be q, r, b, or n".into()),
        }
    } else {
        None
    };
    Ok(ChessMove::new(from, to, promotion))
}

fn square(raw: &str) -> Result<Square, String> {
    let bytes = raw.as_bytes();
    if bytes.len() != 2 {
        return Err("square must have file and rank".into());
    }
    let file = match bytes[0] {
        b'a'..=b'h' => File::from_index((bytes[0] - b'a') as usize),
        _ => return Err("file must be a-h".into()),
    };
    let rank = match bytes[1] {
        b'1'..=b'8' => Rank::from_index((bytes[1] - b'1') as usize),
        _ => return Err("rank must be 1-8".into()),
    };
    Ok(Square::make_square(rank, file))
}

pub fn render_board(board: &Board) -> String {
    let mut out = String::new();
    out.push_str("    a b c d e f g h\n");
    out.push_str("  +-----------------+\n");
    for rank_idx in (0..8).rev() {
        let rank = Rank::from_index(rank_idx);
        out.push_str(&format!("{} |", rank_idx + 1));
        for file_idx in 0..8 {
            let square = Square::make_square(rank, File::from_index(file_idx));
            let glyph = match board.piece_on(square) {
                Some(piece) => {
                    let base = match piece {
                        Piece::Pawn => 'P',
                        Piece::Knight => 'N',
                        Piece::Bishop => 'B',
                        Piece::Rook => 'R',
                        Piece::Queen => 'Q',
                        Piece::King => 'K',
                    };
                    match board.color_on(square) {
                        Some(Color::White) => base,
                        Some(Color::Black) => base.to_ascii_lowercase(),
                        None => base,
                    }
                }
                None => '.',
            };
            out.push(' ');
            out.push(glyph);
        }
        out.push_str(" |\n");
    }
    out.push_str("  +-----------------+\n");
    out.push_str("    a b c d e f g h\n");
    let side = match board.side_to_move() {
        Color::White => "White",
        Color::Black => "Black",
    };
    out.push_str(&format!("Side to move: {side}\n"));
    out
}

#[derive(Default)]
pub struct OmnidexProvider {
    pub command: Option<String>,
}

impl MoveProvider for OmnidexProvider {
    fn choose_move(&mut self, state: &GameState) -> Result<String, String> {
        let legal = state.legal_uci_moves();
        let command = self.command.clone().unwrap_or_else(|| "/home/gryph/.omnidex/bin/omni".to_string());
        let prompt = format!(
            "Choose one legal chess move for the side to move. Board FEN: {}. Legal UCI moves: {}. Return exactly one UCI move from the legal list and no other text.",
            state.board,
            legal.join(", ")
        );
        let output = Command::new(command)
            .args(["run", "-permission", "full_access", "-no-permission-prompt"])
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .and_then(|mut child| {
                use std::io::Write;
                child.stdin.as_mut().expect("stdin piped").write_all(prompt.as_bytes())?;
                child.wait_with_output()
            })
            .map_err(|err| format!("failed to invoke Omnidex: {err}"))?;
        let stdout = String::from_utf8_lossy(&output.stdout);
        extract_legal_move(&stdout, &legal)
            .ok_or_else(|| format!("Omnidex did not return a legal move. Output: {stdout}"))
    }
}

pub fn extract_legal_move(text: &str, legal: &[String]) -> Option<String> {
    for token in text.split(|ch: char| !ch.is_ascii_alphanumeric()) {
        let candidate = token.trim().to_lowercase();
        if legal.iter().any(|mv| mv == &candidate) {
            return Some(candidate);
        }
    }
    None
}

pub fn play_engine_turn<P: MoveProvider>(state: &mut GameState, provider: &mut P) -> Result<String, String> {
    let mv = provider.choose_move(state)?;
    state.apply_uci(&mv)?;
    Ok(mv)
}

pub fn should_play_again(input: &str) -> bool {
    input.trim().eq_ignore_ascii_case("y") || input.trim().eq_ignore_ascii_case("yes")
}

#[cfg(test)]
mod tests {
    use super::*;

    struct ScriptedProvider(&'static str);
    impl MoveProvider for ScriptedProvider {
        fn choose_move(&mut self, _state: &GameState) -> Result<String, String> {
            Ok(self.0.to_string())
        }
    }

    #[test]
    fn legal_move_is_applied() {
        let mut state = GameState::default();
        state.apply_uci("e2e4").unwrap();
        assert_ne!(state.board, Board::default());
    }

    #[test]
    fn illegal_move_is_rejected() {
        let mut state = GameState::default();
        assert!(state.apply_uci("e2e5").is_err());
    }

    #[test]
    fn omnidex_move_provider_output_is_validated() {
        let mut state = GameState::default();
        let mut bad = ScriptedProvider("e2e5");
        assert!(play_engine_turn(&mut state, &mut bad).is_err());
        let mut good = ScriptedProvider("e2e4");
        assert!(play_engine_turn(&mut state, &mut good).is_ok());
    }

    #[test]
    fn extracts_only_legal_omnidex_move() {
        let legal = vec!["e2e4".to_string(), "g1f3".to_string()];
        assert_eq!(extract_legal_move("I choose e2e4", &legal), Some("e2e4".to_string()));
        assert_eq!(extract_legal_move("e2e5", &legal), None);
    }

    #[test]
    fn board_rendering_is_human_readable() {
        let rendered = render_board(&Board::default());
        assert!(rendered.contains("    a b c d e f g h"));
        assert!(rendered.contains("8 | r n b q k b n r |"));
        assert!(rendered.contains("1 | R N B Q K B N R |"));
        assert!(rendered.contains("Side to move: White"));
    }

    #[test]
    fn game_status_has_end_screen_text() {
        let state = GameState::default();
        assert_eq!(state.status_text(), "Game in progress");
    }

    #[test]
    fn play_again_flow_accepts_yes_only() {
        assert!(should_play_again("y"));
        assert!(should_play_again(" yes "));
        assert!(!should_play_again("n"));
        assert!(!should_play_again(""));
    }
}
