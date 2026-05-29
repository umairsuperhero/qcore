export const learningDictionary: Record<string, string> = {
  "Sent S1 Setup Request": `**S1 Setup Request**  
The eNodeB (cell tower) introduces itself to the MME. It announces its supported Tracking Area Codes (TAC) and Public Land Mobile Networks (PLMNs) so the MME knows which subscribers can connect here.`,
  "Received S1 Setup Response": `**S1 Setup Response**  
The MME accepts the eNodeB. The S1-MME control plane connection is now established and ready for UE traffic.`,
  "Sent NAS Attach Request": `**Attach Request**  
The UE sends its first message to the core network, saying "I want to connect." It includes its IMSI (SIM card ID) and security capabilities.`,
  "Received NAS Authentication Request": `**Authentication Request**  
The core network challenges the UE to prove it has the correct SIM card. The core sends a random number (RAND) and an Authentication Token (AUTN) that only the true SIM and the core can decode.`,
  "Sent NAS Authentication Response": `**Authentication Response**  
The SIM card runs the crypto algorithms (Milenage) using the RAND and its secret key (Ki). It returns the expected result (RES). If RES matches what the core expects, authentication succeeds!`,
  "Received NAS Security Mode Command": `**Security Mode Command**  
The core tells the UE what encryption and integrity algorithms to use for the rest of the conversation.`,
  "Sent NAS Security Mode Complete": `**Security Mode Complete**  
The UE acknowledges the security settings. From this point forward, all NAS messages are encrypted and integrity-protected.`,
  "Received Initial Context Setup Request": `**Initial Context Setup**  
The core tells the eNodeB to establish a radio bearer for the UE and prepare to route its user-plane data (internet traffic).`,
  "Sent Initial Context Setup Response": `**Initial Context Setup Response**  
The eNodeB confirms the radio bearer is set up.`,
  "Sent NAS Attach Complete": `**Attach Complete**  
The UE confirms that the radio bearer is active. The UE is now fully attached and can send/receive internet traffic!`,
  "Received NAS EMM Information": `**EMM Information**  
An optional message where the core can send the UE extra details, like the local time zone or network name.`,
};

export function getLearningContent(message: string): string | null {
  for (const [key, content] of Object.entries(learningDictionary)) {
    if (message.includes(key)) {
      return content;
    }
  }
  return null;
}
