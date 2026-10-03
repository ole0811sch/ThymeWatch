use std::time::{Duration, SystemTime};

use gtk::gdk::Display;
use gtk::{Application, ApplicationWindow, Button, CssProvider, Entry, glib};
use gtk::{Widget, prelude::*};

fn main() -> glib::ExitCode {
    let application = Box::leak(Box::new(
        Application::builder()
            .application_id("com.example.FirstGtkApp")
            .build(),
    ));

    application.connect_startup(|_app| {
        let provider = CssProvider::new();
        provider.load_from_string(
            "
:root {
    background-color: rgba(0, 0, 0, 0);
}
",
        );

        // Add the provider to the default screen
        gtk::style_context_add_provider_for_display(
            &Display::default().expect("Could not connect to a display."),
            &provider,
            gtk::STYLE_PROVIDER_PRIORITY_APPLICATION,
        );
    });
    application.connect_activate(|app| {
        let window = ApplicationWindow::builder()
            .application(app)
            .title("First GTK Program")
            .default_width(350)
            .default_height(70)
            .show_menubar(false)
            .decorated(false)
            .build();
        window.set_titlebar(None::<&Widget>);

        let entry = Entry::new();
        entry.connect_activate(|entry| {
            set_alarm(Into::<String>::into(entry.buffer().text())).unwrap();
            eprintln!("Clicked! {}", Into::<String>::into(entry.buffer().text()));
            application.windows().iter().for_each(|w| w.close());
        });
        window.set_child(Some(&entry));

        window.present();
    });

    application.run()
}

fn set_alarm(s: String) -> Result<(), Box<dyn std::error::Error>> {
    let minutes: f64 = s.parse()?;
    let millis = (SystemTime::now() + Duration::try_from_secs_f64(minutes * 60.0).unwrap())
        .duration_since(SystemTime::UNIX_EPOCH)
        .unwrap_or(Duration::ZERO)
        .as_secs_f64()
        * 1000.0;
    let response = minreq::post("http://localhost:8091/alarm")
        .with_param("expirytime", millis.to_string())
        .send()?;
    assert_eq!(200, response.status_code);

    Ok(())
}
