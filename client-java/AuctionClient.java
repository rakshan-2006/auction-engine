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

        SSLSocket socket = (SSLSocket) factory.createSocket("localhost", 8080);

        BufferedReader input = new BufferedReader(new InputStreamReader(System.in));
        BufferedReader server = new BufferedReader(new InputStreamReader(socket.getInputStream()));
        PrintWriter out = new PrintWriter(socket.getOutputStream(), true);

        System.out.println("Enter bidder name:");
        String name = input.readLine();

        while (true) {

            System.out.println("Enter bid amount:");

            String bid = input.readLine();

            out.println("BID " + name + " " + bid);

            String response = server.readLine();

            System.out.println("Server: " + response);
        }
    }
}
