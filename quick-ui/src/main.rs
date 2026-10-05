use std::sync::LazyLock;
use std::time::{Duration, SystemTime};

use async_channel;
use glib::{Propagation, clone};
use gtk::gdk::{Display, ModifierType};
use gtk::{
    Application, ApplicationWindow, CssProvider, Entry, EventControllerKey, Label, ListBox,
    ListBoxRow, glib,
};
use gtk::{Widget, prelude::*};
use reqwest::StatusCode;
use tokio::runtime::Runtime;
use tokio::time::{sleep};

static RUNTIME: LazyLock<Runtime> = LazyLock::new(|| Runtime::new().unwrap());

enum RequestState {
    Started,
    Response(Result<(), String>),
    Close,
}

fn main() -> glib::ExitCode {
    let application = Application::builder()
        .application_id("com.example.FirstGtkApp")
        .build();

    application.connect_startup(|_app| {
        let provider = CssProvider::new();
        provider.load_from_string(
            "
:root, #list-box {
    background-color: rgba(0, 0, 0, 0);
}
label {
    background-color: red;
    border-style: none;
    border-radius: 1em;
}
label.success {
    background-color: green;
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
    application.connect_activate(clone!(
        #[strong]
        application,
        move |app| {
            let window = ApplicationWindow::builder()
                .application(app)
                .title("First GTK Program")
                .default_width(350)
                .default_height(70)
                .show_menubar(false)
                .decorated(false)
                .build();
            window.set_titlebar(None::<&Widget>);
            let (sender, receiver) = async_channel::bounded::<RequestState>(1);

            let list_box = ListBox::new();
            list_box.set_widget_name("list-box");
            let label = Label::new(Some("Enter delay in minutes"));
            let label_row = ListBoxRow::builder()
                .selectable(false)
                .child(&label)
                .build();
            list_box.append(&label_row);
            let entry = Entry::new();
            entry.connect_activate(clone!(
                #[strong]
                label,
                move |entry| {
                    let Ok(minutes) = entry.buffer().text().parse::<f64>() else {
                        label.set_label("Invalid number");
                        return;
                    };
                    if minutes < 0.0 || minutes.is_infinite() || minutes.is_nan() {
                        label.set_label("Invalid number");
                        return;
                    }
                    RUNTIME.spawn(clone!(
                        #[strong]
                        sender,
                        async move {
                            sender.send(RequestState::Started).await.unwrap();
                            let status_code = set_alarm(minutes)
                                .await
                                .map_err(|e| format!("{e}"))
                                .and_then(|code| {
                                    if code == StatusCode::OK {
                                        Ok(())
                                    } else {
                                        Err(format!("Failure. Server returned status code {code}"))
                                    }
                                });
                            let is_ok = status_code.is_ok();
                            sender
                                .send(RequestState::Response(status_code))
                                .await
                                .unwrap();
                            if is_ok {
                                sleep(Duration::from_secs_f32(0.5)).await;
                                sender.send(RequestState::Close).await.unwrap();
                            }
                        }
                    ));
                }
            ));
            glib::spawn_future_local(clone!(
                #[weak]
                application,
                #[weak]
                entry,
                async move {
                    loop {
                        let response = receiver.recv().await.unwrap();
                        match response {
                            RequestState::Started => {
                                entry.set_sensitive(false);
                            }
                            RequestState::Response(response) => match response {
                                Ok(_) => {
                                    eprintln!(
                                        "Clicked! {}",
                                        Into::<String>::into(entry.buffer().text())
                                    );
                                    label.set_label("Timer was set");
                                    label.add_css_class("success");
                                }
                                Err(msg) => {
                                    entry.set_sensitive(true);
                                    entry.grab_focus();
                                    label.set_label(&msg);
                                }
                            },
                            RequestState::Close => {
                                application.windows().iter().for_each(|w| w.close());
                            }
                        }
                    }
                }
            ));
            let controller = EventControllerKey::new();
            controller.connect_key_pressed(clone!(
                #[strong]
                application,
                move |_controller, keyval, _keycode, state| {
                    if [
                        ModifierType::SHIFT_MASK,
                        ModifierType::SUPER_MASK,
                        ModifierType::META_MASK,
                        ModifierType::HYPER_MASK,
                    ]
                    .iter()
                    .all(|m| !state.contains(*m))
                        && keyval.name().is_some_and(|keyval| {
                            let name = String::from(keyval);
                            state.contains(ModifierType::CONTROL_MASK)
                                && ["c", "d"].contains(&name.as_str())
                                || !state.contains(ModifierType::CONTROL_MASK) && name == "Escape"
                        })
                    {
                        application.windows().iter().for_each(|w| w.close());

                        return Propagation::Stop;
                    }

                    Propagation::Proceed
                }
            ));
            controller.set_propagation_phase(gtk::PropagationPhase::Capture);
            entry.add_controller(controller);
            let entry_row = ListBoxRow::builder()
                .selectable(false)
                .child(&entry)
                .build();
            list_box.append(&entry_row);
            window.set_child(Some(&list_box));

            window.present();
            entry.grab_focus();
        }
    ));

    application.run()
}

async fn set_alarm(minutes: f64) -> Result<StatusCode, Box<dyn std::error::Error>> {
    let millis = (SystemTime::now() + Duration::try_from_secs_f64(minutes * 60.0).unwrap())
        .duration_since(SystemTime::UNIX_EPOCH)
        .unwrap_or(Duration::ZERO)
        .as_secs_f64()
        * 1000.0;
    let response = reqwest::Client::new()
        .post("http://localhost:8091/alarm")
        .query(&[("expirytime", millis.to_string())])
        .send()
        .await?;

    Ok(response.status())
}
