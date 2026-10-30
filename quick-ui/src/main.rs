use std::process::{Command, Stdio};
use std::sync::mpsc::Receiver;
use std::sync::{LazyLock, mpsc};
use std::time::Duration;

use async_channel;
use glib::{Propagation, clone};
use gtk::gdk::{Display as GDKDisplay, ModifierType};
use gtk::{
    Application, ApplicationWindow, CssProvider, Entry, EventControllerKey, Label, ListBox,
    ListBoxRow, glib,
};
use gtk::{Widget, prelude::*};
use tokio::runtime::Runtime;
use tokio::time::sleep;

use crate::websocket::RequestError::TungsteniteError;
use crate::websocket::set_alarm_ws;
pub mod websocket;

static RUNTIME: LazyLock<Runtime> = LazyLock::new(|| Runtime::new().unwrap());

enum RequestState {
    Started,
    Response(Result<(), String>),
    LaunchingServer,
    Close,
}

const SERVER_CMD: [&str; 3] = [
    "sh",
    "-c",
    "cd /home/ole/Documents/code/ThymeWatch/daemon && go run .",
];

fn launch(cmd: &'static [&'static str]) -> Result<Receiver<()>, ()> {
    let Ok(mut child) = Command::new(cmd[0])
        .args(&cmd[1..])
        .stdin(Stdio::null())
        .stdout(Stdio::inherit())
        .stderr(Stdio::inherit())
        .spawn()
    else {
        return Err(());
    };
    let (sender, receiver) = mpsc::channel::<()>();
    gio::spawn_blocking(move || {
        let _ = child.wait();
        sender.send(()).unwrap()
    });

    Ok(receiver)
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
            &GDKDisplay::default().expect("Could not connect to a display."),
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
                            let mut recv_server_terminated: Option<Receiver<()>> = None;
                            let response = loop {
                                sender.send(RequestState::Started).await.unwrap();
                                let result = set_alarm_ws(minutes).await;
                                let err = match result {
                                    Ok(_) => break Ok(()),
                                    Err(e) => e,
                                };
                                if match &err {
                                    TungsteniteError(
                                        tokio_tungstenite::tungstenite::Error::Io(e),
                                    ) => {
                                        dbg!(&e);
                                        match e.kind() {
                                            std::io::ErrorKind::ConnectionRefused => true,
                                            _ => false,
                                        }
                                    }
                                    _ => false,
                                } {
                                    sender.send(RequestState::LaunchingServer).await.unwrap();
                                    match recv_server_terminated {
                                        Some(ref recv) => match recv.try_recv() {
                                            Ok(_) => {
                                                break Err("Server terminated".to_string());
                                            }
                                            Err(mpsc::TryRecvError::Empty) => continue,
                                            res => res.unwrap(),
                                        },
                                        _ => (),
                                    };
                                    match launch(&SERVER_CMD) {
                                        Ok(receiver) => {
                                            recv_server_terminated = Some(receiver);
                                            continue;
                                        }
                                        Err(_) => {
                                            break Err("Could not launch server".to_string());
                                        }
                                    }
                                } else {
                                    break Err(format!("{}", err.into_value()));
                                }
                            };

                            let is_ok = response.is_ok();
                            sender.send(RequestState::Response(response)).await.unwrap();
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
                            RequestState::LaunchingServer => {
                                label.set_label("Launching ThymeWatch");
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
