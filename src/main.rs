use clap::{Arg, ArgMatches, Command, command};

// enum Command {
//     Init,
//     List,
//     Add,
//     Edit,
//     Delete,
//     Summary,
//     Config
// }

fn main() {
    let match_results = command!()
    .version("0.1.0")
    .about("A command-line tool for developers to track daily activities and generate formatted timesheet summaries.")
    .subcommand(Command::new("init"))
    .subcommand(Command::new("list").about("Displays logged entries with optional filters"))
    .subcommand(Command::new("add").about("Logs a new activity entry"))
    .subcommand(Command::new("edit").about("Opens an interactive prompt to modify an existing entry"))
    .subcommand(Command::new("delete").about("Removes an entry after confirmation"))
    .subcommand(Command::new("summary").about("Generates a structured summary from logged entries"))
    .subcommand(Command::new("config").about("Reads and writes configuration values"))
    .get_matches();

    match match_results.subcommand() {
        Some(("list", sub_matches)) => {
            println!("user ran list")
        }
        _ => {}
    }
}
