use clap::{Arg, ArgMatches, command};


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
    let matches = command!()
    .about("A command-line tool for developers to track daily activities and generate formatted timesheet summaries.")
    .arg(
        Arg::new("init")
            .short('i')
            .long("init")
            .help("Provides a config file")
    )
    .arg(
        Arg::new("list")
            .short('l')
            .long("list")
            .help("Displays logged entries with optional filters")
    )
    .arg(
        Arg::new("add")
            .short('a')
            .long("add")
            .help("Logs a new activity entry")
    )
    .arg(
        Arg::new("edit")
            .short('e')
            .long("edit")
    )
    .arg(
        Arg::new("delete")
            .short('d')
            .long("delete")
    )
    .arg(
        Arg::new("summary")
            .short('s')
            .long("summary")
    )
    .arg(
        Arg::new("config")
            .short('c')
            .long("config")
    )
    .get_matches();
}
