const leaderEl = document.getElementById("leader");
const highestBidEl = document.getElementById("highestBid");
const countdownEl = document.getElementById("countdown");
const extensionsEl = document.getElementById("extensions");
const statusTagEl = document.getElementById("statusTag");
const formMessageEl = document.getElementById("formMessage");
const bidFormEl = document.getElementById("bidForm");
const bidderEl = document.getElementById("bidder");
const amountEl = document.getElementById("amount");
const eventFeedEl = document.getElementById("eventFeed");

const state = {
    highestBid: 0,
    highestBidder: "",
    remainingMs: 0,
    extensionCount: 0,
    auctionFinalized: false,
    ws: null,
};

function formatMs(ms) {
    const safe = Math.max(0, Number(ms || 0));
    const totalSeconds = Math.floor(safe / 1000);
    const minutes = Math.floor(totalSeconds / 60)
        .toString()
        .padStart(2, "0");
    const seconds = (totalSeconds % 60).toString().padStart(2, "0");
    return `${minutes}:${seconds}`;
}

function pushEvent(text, isError = false) {
    const li = document.createElement("li");
    if (isError) {
        li.classList.add("error");
    }
    li.innerHTML = `<strong>${new Date().toLocaleTimeString()}</strong> ${text}`;
    eventFeedEl.prepend(li);
    while (eventFeedEl.children.length > 60) {
        eventFeedEl.removeChild(eventFeedEl.lastChild);
    }
}

function render() {
    leaderEl.textContent = state.highestBidder || "None yet";
    highestBidEl.textContent = String(state.highestBid || 0);
    countdownEl.textContent = formatMs(state.remainingMs);
    extensionsEl.textContent = String(state.extensionCount || 0);

    if (state.auctionFinalized) {
        statusTagEl.textContent = "Auction closed";
        statusTagEl.style.background = "#f6d7d7";
    }
}

function setConnected(connected) {
    if (!state.auctionFinalized) {
        statusTagEl.textContent = connected ? "Live sync active" : "Reconnecting...";
        statusTagEl.style.background = connected ? "#d6f2ea" : "#ede8de";
    }
}

function applyState(payload) {
    state.highestBid = payload.highestBid || 0;
    state.highestBidder = payload.highestBidder || "";
    state.remainingMs = payload.remainingMs || 0;
    state.extensionCount = payload.extensionCount || 0;
    state.auctionFinalized = Boolean(payload.auctionFinalized);
    render();
}

function connect() {
    const protocol = window.location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${protocol}://${window.location.host}/ws`);
    state.ws = ws;

    ws.onopen = () => {
        setConnected(true);
        pushEvent("WebSocket connected to auction server.");
    };

    ws.onclose = () => {
        setConnected(false);
        pushEvent("WebSocket disconnected. Retrying in 1.5s...", true);
        setTimeout(connect, 1500);
    };

    ws.onerror = () => {
        setConnected(false);
    };

    ws.onmessage = (event) => {
        try {
            const payload = JSON.parse(event.data);
            switch (payload.type) {
                case "STATE":
                    applyState(payload);
                    break;
                case "NEW_HIGHEST":
                    state.highestBid = payload.amount || state.highestBid;
                    state.highestBidder = payload.bidder || state.highestBidder;
                    if (payload.auctionEndsAt) {
                        const endMs = new Date(payload.auctionEndsAt).getTime();
                        state.remainingMs = Math.max(0, endMs - Date.now());
                    }
                    pushEvent(`New highest bid by ${state.highestBidder}: ${state.highestBid}`);
                    render();
                    break;
                case "BID_ACCEPTED":
                    formMessageEl.textContent = `Accepted. Leader: ${payload.highestBidder} @ ${payload.highestBid}`;
                    formMessageEl.style.color = "#1f8a70";
                    break;
                case "BID_REJECTED":
                    formMessageEl.textContent = `Rejected: ${payload.message}`;
                    formMessageEl.style.color = "#b00020";
                    pushEvent(`Bid rejected. ${payload.message}`, true);
                    if (payload.auctionEndsAt) {
                        const endMs = new Date(payload.auctionEndsAt).getTime();
                        state.remainingMs = Math.max(0, endMs - Date.now());
                    }
                    state.highestBid = payload.highestBid || state.highestBid;
                    state.highestBidder = payload.highestBidder || state.highestBidder;
                    render();
                    break;
                case "AUCTION_CLOSED":
                    state.auctionFinalized = true;
                    pushEvent(`Auction closed. Winner: ${payload.winner || "None"} @ ${payload.amount || 0}`);
                    render();
                    break;
                default:
                    break;
            }
        } catch (_err) {
            pushEvent("Received non-JSON message from server.", true);
        }
    };
}

bidFormEl.addEventListener("submit", (event) => {
    event.preventDefault();

    if (!state.ws || state.ws.readyState !== WebSocket.OPEN) {
        formMessageEl.textContent = "Connection not ready. Please wait...";
        formMessageEl.style.color = "#b00020";
        return;
    }

    const bidder = bidderEl.value.trim();
    const amount = Number(amountEl.value);

    state.ws.send(
        JSON.stringify({
            type: "BID",
            bidder,
            amount,
        })
    );

    formMessageEl.textContent = "Bid sent...";
    formMessageEl.style.color = "#5f6770";
});

setInterval(() => {
    if (state.remainingMs > 0) {
        state.remainingMs = Math.max(0, state.remainingMs - 250);
        render();
    }
}, 250);

render();
connect();
