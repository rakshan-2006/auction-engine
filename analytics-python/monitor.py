import time

bids = []

print("Auction Monitor Started")

while True:
    bid = input("Enter bid amount for logging: ")

    timestamp = time.time()

    bids.append((bid, timestamp))

    print("Total bids:", len(bids))
