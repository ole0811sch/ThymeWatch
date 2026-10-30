use std::fmt::{Debug, Display};

use RequestError::{InternalError, TungsteniteError};
use futures_util::{SinkExt, StreamExt};
use serde::{Deserialize, Serialize};
use time::OffsetDateTime;
use tokio_tungstenite::connect_async;
use tokio_tungstenite::tungstenite::{Bytes, Message};

#[derive(Serialize, Deserialize)]
struct MessageHeader<'a> {
    #[serde(rename = "inReplyTo", default)]
    in_reply_to: Option<f64>,
    #[serde(default)]
    tag: Option<&'a str>,
    id: f64,
}

#[derive(Serialize, Deserialize)]
struct CreateAlarmMessage<'a> {
    #[serde(rename = "expiryTime", with = "time::serde::rfc3339")]
    expiry_time: time::OffsetDateTime,
    #[serde(default)]
    name: Option<&'a str>,
    #[serde(rename = "startedAt", with = "time::serde::rfc3339")]
    started_at: time::OffsetDateTime,
}

#[derive(Serialize)]
struct DismissAlarmsMessage {
    ids: Vec<f64>,
}

#[derive(Serialize, Deserialize, Debug)]
struct AlarmCreatedMessage {
    id: f64,
}

#[derive(Deserialize, Debug)]
struct StatusMessage {
    status: i32,
}

pub struct StringError {
    msg: String,
}

impl Display for StringError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(self.msg.as_str())
    }
}
impl std::fmt::Debug for StringError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        Display::fmt(&self, f)
    }
}

impl std::error::Error for StringError {}

pub enum RequestError {
    InternalError(Box<dyn std::error::Error + Send>),
    TungsteniteError(tokio_tungstenite::tungstenite::Error),
}

impl RequestError {
    pub fn into_value(self: Self) -> Box<dyn std::error::Error> {
        match self {
            InternalError(e) => e,
            TungsteniteError(e) => Box::new(e),
        }
    }
}

impl<T> From<T> for RequestError
where
    T: std::error::Error + 'static + Send,
{
    fn from(value: T) -> Self {
        InternalError(Box::new(value))
    }
}

pub async fn set_alarm_ws(minutes: f64) -> Result<(), RequestError> {
    let now = OffsetDateTime::now_utc();
    let expiry_time = match OffsetDateTime::from_unix_timestamp_nanos(
        now.unix_timestamp_nanos() + (minutes * 60.0 * 1e9) as i128,
    ) {
        Ok(v) => v,
        Err(e) => return Err(InternalError(Box::new(e))),
    };
    let body = CreateAlarmMessage {
        expiry_time: expiry_time,
        started_at: now,
        name: None,
    };
    let _response: AlarmCreatedMessage = make_request("CREATE_ALARM", body).await?;

    Ok(())
}

pub async fn dismiss_alarms() -> Result<(), RequestError> {
    let response: StatusMessage =
        make_request("DISMISS_ALARMS", DismissAlarmsMessage { ids: vec![] }).await?;

    if response.status != 200 {
        return Err(InternalError(Box::new(StringError {
            msg: format!("Server returned status {}", response.status),
        })));
    }

    Ok(())
}

pub async fn make_request<RequestT: Serialize, ResponseT: for<'a> Deserialize<'a>>(
    tag: &str,
    body: RequestT,
) -> Result<ResponseT, RequestError> {
    let mut ws = match connect_async("ws://localhost:8080").await {
        Ok((ws, _)) => ws,
        Err(e) => return Err(TungsteniteError(e)),
    };
    let header = MessageHeader {
        id: 0.0,
        in_reply_to: None,
        tag: Some(tag),
    };
    let msg =
        serde_json::ser::to_string(&header)? + "\n" + serde_json::ser::to_string(&body)?.as_str();
    let _ = ws.send(Message::Text(msg.into())).await;

    let response: ResponseT = loop {
        let Some(msg) = ws.next().await else {
            return Err(InternalError(Box::new(StringError {
                msg: "Connection closed before request was handled".to_string(),
            })));
        };
        let msg = msg?;
        if msg.is_ping() {
            ws.send(Message::Pong(Bytes::new())).await?;
        }
        if let Message::Close(frame) = msg {
            Err(StringError {
                msg: format!(
                    "Server closed connection: {}",
                    if let Some(frame) = frame {
                        format!("code={}, reason={}", frame.code, frame.reason)
                    } else {
                        "No close frame".to_string()
                    }
                ),
            })?;
            unreachable!()
        }
        if msg.is_binary() {
            return Err(InternalError(Box::new(StringError {
                msg: "Server send binary data".to_string(),
            })));
        }
        if msg.is_pong() {
            continue;
        }
        if msg.is_empty() {
            continue;
        }
        if !msg.is_text() {
            continue;
        }
        let Message::Text(msg) = msg else {
            return Err(InternalError(Box::new(StringError {
                msg: "Unknown message type".to_string(),
            })));
        };

        let Some((header_str, body_str)) = msg.split_once('\n') else {
            Err(StringError {
                msg: "Server sent invalid message".to_string(),
            })?;
            unreachable!();
        };
        let header: MessageHeader = serde_json::from_str(header_str)?;
        match header.in_reply_to {
            Some(0.0) => (),
            _ => continue,
        }
        let body: ResponseT = serde_json::from_str(body_str)?;
        break body;
    };

    if let Err(e) = ws.close(None).await {
        eprintln!("Error while closing: {e}")
    }

    Ok(response)
}
