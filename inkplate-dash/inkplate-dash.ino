#include <Inkplate.h>

/*
   Inkplate10_Show_JPG_With_HTTPClient example for Soldered Inkplate 10
   For this example you will need a USB-C cable, Inkplate 10, and an available WiFi connection.
   Select "e-radionica Inkplate10" or "Soldered Inkplate10" from Tools -> Board menu.
   Don't have "e-radionica Inkplate10" or "Soldered Inkplate10" option? Follow our tutorial and add it:
   https://soldered.com/learn/add-inkplate-6-board-definition-to-arduino-ide/

   This example will show you how to show JPG image using HTTPClient.
   Make sure that you entered WiFi credentials and change the image link if you want any other image.

   Want to learn more about Inkplate? Visit www.inkplate.io
   Looking to get support? Write on our forums: https://forum.soldered.com/
   17 April 2023 by Soldered
*/

// Next 3 lines are a precaution, you can ignore those, and the example would also work without them
#if !defined(ARDUINO_INKPLATE10) && !defined(ARDUINO_INKPLATE10V2)
#error                                                                                                                 \
    "Wrong board selection for this example, please select e-radionica Inkplate10 or Soldered Inkplate10 in the boards menu."
#endif

// Include needed libraries in the sketch
#include "HTTPClient.h"
#include "Inkplate.h"
#include "WiFi.h"
#include "esp_task_wdt.h"

// Create an object on Inkplate library and also set library into 1 Bit mode (BW)
Inkplate display(INKPLATE_1BIT);

/**************** CHANGE HERE ******************/
#include "config_secret.h"  // Copy config_secret.h.example to config_secret.h and add ssid, pass, url

// Here you can change the interval of updating the image.
#define UPDATE_INTERVAL_IN_SESCS 300

// Retry settings for image loading with exponential backoff (for HTTP/network failures)
#define MAX_RETRIES 5
#define RETRY_MIN_DELAY_MS 2000UL   // Start with 2 seconds
#define RETRY_MAX_DELAY_MS 60000UL  // Cap at 60 seconds
#define RETRY_BACKOFF_MULTIPLIER 2UL // Double the delay each time
#define OOM_RETRY_DELAY_MS 2000UL   // Fixed short delay for alloc failure (backoff doesn't help OOM)

// Watchdog timeout in seconds. 120s is enough: we reset after each http.GET() (max 60s)
// and every 2s during backoff, so we never go >60s between resets in the retry path.
#define WDT_TIMEOUT_SECS 120

// Reset WDT at least this often during long delays (backoff) so we never exceed WDT timeout
#define WDT_RESET_INTERVAL_MS 2000

// Reject responses larger than this (getSize() can be wrong or malicious); dashboard image is typically much smaller
#define MAX_IMAGE_SIZE (2 * 1024 * 1024)

/***********************************************/

// Variable that holds last connection time
unsigned long lastConnectionTime = 0;

// Track if we've successfully loaded an image at least once
bool hasLoadedImage = false;


void setup()
{
    display.begin();             // Init Inkplate library (you should call this function ONLY ONCE)
    display.clearDisplay();      // Clear frame buffer of display
    display.setTextSize(2);      // Set text size to be 2 times bigger than original (5x7 px)
    display.setTextColor(BLACK); // Set text color to black

    Serial.begin(115200);

    // Initialize watchdog timer
    esp_task_wdt_init(WDT_TIMEOUT_SECS, true); // Enable panic so ESP32 restarts
    esp_task_wdt_add(NULL);                    // Add current thread to WDT watch
    Serial.println("Watchdog initialized with " + String(WDT_TIMEOUT_SECS) + "s timeout");

    // Let's connect to the WiFi
    // Show a connection message
    display.print("Connecting to WiFi");
    // We can use a partial update because Inkplate is in 1-bit mode
    display.partialUpdate();

    // Actually connect to the WiFi network
    WiFi.mode(WIFI_MODE_STA);
    WiFi.begin(ssid, pass);
    while (WiFi.status() != WL_CONNECTED)
    {
        // Print a dot every half second when connecting
        delay(500);
        display.print(".");
        display.partialUpdate();
        esp_task_wdt_reset(); // Keep watchdog happy during WiFi connection
    }
    display.println("\nWiFi OK! Downloading...");
    display.partialUpdate();

    Serial.println();
    Serial.print("Connected to WiFi network with IP Address: ");
    Serial.println(WiFi.localIP());

    // Switch to 3-bit mode so the image will be of better quality
    // NOTE: You can't use partial update when the Inkplate is in the 3-bit mode!
    display.setDisplayMode(INKPLATE_3BIT);
    display.clearDisplay();
    display.display();
    getandprintdash();
}

void loop()
{
    // Reset watchdog timer
    esp_task_wdt_reset();

    // Every POSTING_INTERVAL_IN_SESCS seconds make the POST request
    if ((unsigned long)(millis() - lastConnectionTime) > UPDATE_INTERVAL_IN_SESCS * 1000LL)
    {
      getandprintdash();
    }
    else {
      delay(1000);
    }
}

// Delay for ms milliseconds, resetting WDT periodically so long backoffs don't trigger the watchdog.
static void delayWithWdtReset(unsigned long ms) {
    unsigned long elapsed = 0;
    while (elapsed < ms) {
        unsigned long chunk = (ms - elapsed) > WDT_RESET_INTERVAL_MS ? WDT_RESET_INTERVAL_MS : (ms - elapsed);
        delay(chunk);
        elapsed += chunk;
        esp_task_wdt_reset();
    }
}

void getandprintdash() {
    bool success = false;
    int lastHttpCode = 0;
    int32_t lastSize = 0;
    unsigned long retryDelay = RETRY_MIN_DELAY_MS;

    for (int attempt = 1; attempt <= MAX_RETRIES && !success; attempt++) {
        esp_task_wdt_reset(); // Feed WDT at start of each retry (before long GET)
        Serial.println("Attempt " + String(attempt) + " of " + String(MAX_RETRIES));

        // Make an object for the HTTP client
        HTTPClient http;
        http.begin(url);
        http.setTimeout(60000);

        // Do a get request to get the image
        int httpCode = http.GET();
        esp_task_wdt_reset(); // Feed WDT after GET returns (GET can take up to 60s)
        lastHttpCode = httpCode;

        // If everything is OK
        if (httpCode == HTTP_CODE_OK)
        {
            // getSize() returns -1 when Content-Length is missing (e.g. chunked encoding)
            int32_t size = http.getSize();
            lastSize = size;
            if (size < 0) {
                Serial.println("Invalid response: Content-Length missing or invalid");
            } else if (size == 0) {
                Serial.println("Invalid response length: 0");
            } else if ((uint32_t)size > MAX_IMAGE_SIZE) {
                Serial.println("Response too large: " + String(size) + " (max " + String(MAX_IMAGE_SIZE) + ")");
            } else {
                int32_t len = size; // Copy whose value we will change, but the original must not be lost

                // Allocate the memory for the image
                uint8_t *buffer = (uint8_t *)ps_malloc(size);

                if (buffer == NULL) {
                    Serial.println("Failed to allocate memory for image");
                    http.end();
                    if (attempt < MAX_RETRIES) {
                        // OOM rarely resolves by waiting; use a short fixed delay and don't
                        // increase retryDelay (that's for HTTP backoff only).
                        Serial.println("Retrying in " + String(OOM_RETRY_DELAY_MS / 1000) + " seconds...");
                        delayWithWdtReset(OOM_RETRY_DELAY_MS);
                    }
                    continue;
                }

                uint8_t *buffPtr = buffer; // Copy of the buffer pointer so that the original one is not lost

                // Temporary buffer for retrieving parts of the image and storing them in the real buffer
                uint8_t buff[512] = {0};

                // Let's fetch the data
                WiFiClient *stream = http.getStreamPtr(); // We need a stream pointer to know how much data is available

                // Repeat as long as we have a connection and while there is data to read
                while (http.connected() && (len > 0 || len == -1))
                {
                    // Get the number of available bytes
                    size_t availableBytes = stream->available();

                    // If there are available bytes, read them
                    if (availableBytes)
                    {
                        // Read available bytes from the stream and store them in the buffer
                        int c = stream->readBytes(buff, ((availableBytes > sizeof(buff)) ? sizeof(buff) : availableBytes));
                        memcpy(buffPtr, buff, c);

                        // As we read the data, we subtract the length we read and the remaining length is in the variable
                        // len
                        if (len > 0)
                            len -= c;

                        // Likewise for the buffer pointer
                        buffPtr += c;

                        // Reset watchdog during long downloads
                        esp_task_wdt_reset();
                    }
                    else if (len == -1)
                    {
                        len = 0;
                    }
                }

                // Clear frame buffer first: drawJpegFromBuffer only writes the decoded
                // image rectangle, so any area not covered would show old content.
                display.clearDisplay();
                // Draw image into the frame buffer of Inkplate; free buffer immediately
                // after so we never leak on draw failure or future code changes.
                bool drew = display.drawJpegFromBuffer(buffer, size, 0, 0, true, false);
                free(buffer);
                if (drew) {
                    success = true;
                    hasLoadedImage = true; // Mark that we've successfully loaded an image
                    Serial.println("Image loaded successfully");
                } else {
                    Serial.println("Failed to decode image");
                }
            }
        }
        else
        {
            Serial.println("HTTP error: " + String(httpCode));
        }

        http.end();

        // If not successful and more retries remain, wait before retrying with exponential backoff
        if (!success && attempt < MAX_RETRIES) {
            Serial.println("Retrying in " + String(retryDelay / 1000.0, 1) + " seconds...");
            delayWithWdtReset(retryDelay);
            // Exponential backoff: double the delay for next time, up to max
            retryDelay = min(retryDelay * RETRY_BACKOFF_MULTIPLIER, RETRY_MAX_DELAY_MS);
        }
    }

    // If all retries failed, show error on display
    if (!success) {
        // Only clear display if we've never successfully loaded an image
        // This preserves the previous image when showing errors
        if (!hasLoadedImage) {
            display.clearDisplay();
        }

        // Draw a black rectangle at the top for the error message background
        display.fillRect(0, 0, display.width(), 60, BLACK);

        // Set text to white so it shows on black background
        // Note using 7 here because in 3 bit mode the color range is 0 to 7 and the WHITE definition is 1 which is still nearly black. 
        display.setTextColor(7);
        display.setCursor(5, 5);

        if (lastHttpCode == HTTP_CODE_OK) {
            display.println("ERROR: Invalid length");
            display.setCursor(5, 25);
            display.println(String(lastSize) + " (" + String(MAX_RETRIES) + " attempts)");
        } else {
            display.println("ERROR: HTTP " + String(lastHttpCode));
            display.setCursor(5, 25);
            display.println(String(MAX_RETRIES) + " attempts failed");
        }

        // Reset text color back to black for future use
        display.setTextColor(BLACK);
    }

    // Draw image on the screen
    display.display();
    // Clear frame buffer after flush (does not clear the physical display; next update starts from clean buffer)
    display.clearDisplay();
    lastConnectionTime = millis();
}
