const websocketOpenState = 1;

export const initializationFailureCloseCode = 4001;

type CloseableWebSocket = Pick<WebSocket, "readyState" | "close">;

export function closeAfterInitializationFailure(socket: CloseableWebSocket): void {
  // Browsers only allow clients to send 1000 or application codes in
  // 3000-4999. Protocol codes such as 1008 are reserved for received frames.
  if (socket.readyState === websocketOpenState) {
    socket.close(initializationFailureCloseCode, "initialization failed");
  }
}
