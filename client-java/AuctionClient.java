import javax.net.ssl.*;
import java.io.*;
import java.security.SecureRandom;
import java.security.cert.X509Certificate;

public class AuctionClient {

    public static void main(String[] args) throws Exception {

        TrustManager[] trustAll = new TrustManager[]{
                new X509TrustManager() {
                    public X509Certificate[] getAcceptedIssuers() { return null; }
                    public void checkClientTrusted(X509Certificate[] certs, String authType) {}
                    public void checkServerTrusted(X509Certificate[] certs, String authType) {}
                }
        };

        SSLContext sc = SSLContext.getInstance("TLS");
        sc.init(null, trustAll, new SecureRandom());

        SSLSocketFactory factory = sc.getSocketFactory();
        BufferedReader input = new BufferedReader(new InputStreamReader(System.in));

        System.out.println("Enter server IP or hostname (leave blank for localhost):");
        String serverHost = input.readLine();
        if (serverHost == null || serverHost.trim().isEmpty()) {
            serverHost = "localhost";
        }

        SSLSocket socket = (SSLSocket) factory.createSocket(serverHost.trim(), 8080);

        BufferedReader server = new BufferedReader(new InputStreamReader(socket.getInputStream()));
        PrintWriter out = new PrintWriter(socket.getOutputStream(), true);

        System.out.println("Enter bidder name:");
        String name = input.readLine();

        Thread listenerThread = new Thread(() -> {
            try {
                String response;
                while ((response = server.readLine()) != null) {
                    System.out.println("Server: " + response);
                }
            } catch (IOException e) {
                System.out.println("Disconnected from server.");
            }
        });
        listenerThread.setDaemon(true);
        listenerThread.start();

        while (true) {

            System.out.println("Enter bid amount:");

            String bid = input.readLine();

            out.println("BID " + name + " " + bid);
        }
    }
}
