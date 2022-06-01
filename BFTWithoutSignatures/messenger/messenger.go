package messenger

import (
	"BFTWithoutSignatures/config"
	"BFTWithoutSignatures/logger"
	"BFTWithoutSignatures/threshenc"
	"BFTWithoutSignatures/types"
	"BFTWithoutSignatures/variables"
	"bytes"
	"encoding/gob"
	"time"
	"math"
	"sync"
	"strconv"

	"github.com/pebbe/zmq4"
)

// Sockets
var (
	// Context to initialize sockets
	Context *zmq4.Context

	// SendSockets - Send messages to other servers
	SendSockets map[int]*zmq4.Socket

	// ReceiveSockets - Receive messages from other servers
	ReceiveSockets map[int]*zmq4.Socket

	// ServerSockets - Get the client requests
	ServerSockets map[int]*zmq4.Socket

	// ResponseSockets - Send responses to clients
	ResponseSockets map[int]*zmq4.Socket
)

// Channels for messages
var (
	// MessageChannel - Channel to put the messages that need to be transmitted in
	MessageChannel = make(map[int]chan types.Message)

	// BvbChannel - Channel to put the BVB messages in
	BvbChannel = make(map[int]chan struct {
		BcMessage types.BcMessage
		From      int
	})

	// BcChannel - Channel to put the BC messages in
	BcChannel = make(map[int]chan struct {
		BcMessage types.BcMessage
		From      int
	})

	// RbChannel - Channel to put the RB messages in
	RbChannel = make(map[string]map[int]chan struct {
		RbMessage types.RbMessage
		From      int
	})

	// RbAbcChannel - Channel to put the RB messages for ABC in
	RbAbcChannel = make(chan struct {
		RbMessage types.RbMessage
		From      int
	})

	// MvcChannel - Channel to put the MVC messages in
	MvcChannel = make(map[int]chan struct {
		MvcMessage types.MvcMessage
		From       int
	})

	// VcChannel - Channel to put the VC messages in
	VcChannel = make(map[int]chan struct {
		VcMessage types.VcMessage
		From      int
	})

	// AbcChannel - Channel to put the ABC messages in
	AbcChannel = make(chan struct {
		AbcMessage types.AbcMessage
		From       int
	})

	// RequestChannel - Channel to put the client requests in
	RequestChannel = make(chan []byte, 100)

	// SSVCChannel - Channel to put the Self Stabilized VC messages in
	SSVCChannel = make(map[int]chan struct {
		SSVCMessage types.SSVCMessage
		From      int
	})

	// SSVCDecisionsChannel - Channel to put decided vectors from SSVC
	SSVCDecisionsChannel = make(map[int]chan struct {
		Vector map[int][]byte
		From      int
	})

	// SSABCChannel - Channel to put the Self Stabilized ABC messages in
	SSABCChannel = make(chan struct {
		SSABCMessage types.SSABCMessage
		From      int
	})

)

// Locks for channels in maps (to avoid race conditions)
var (

	BvbMutex = sync.RWMutex{}
	BcMutex = sync.RWMutex{}
	RbMutex = sync.RWMutex{}
	MvcMutex = sync.RWMutex{}
	VcMutex = sync.RWMutex{}
	SSVCMutex = sync.RWMutex{}
	SSVCDecisionsMutex = sync.RWMutex{}

)

// InitializeMessenger - Initializes the 0MQ sockets (between Servers and Clients)
func InitializeMessenger() {
	Context, err := zmq4.NewContext()
	if err != nil {
		logger.ErrLogger.Fatal(err)
	}

	// Initialization of a socket pair to communicate with each one of the other servers
	ReceiveSockets = make(map[int]*zmq4.Socket)
	SendSockets = make(map[int]*zmq4.Socket)
	for i := 0; i < variables.N; i++ {
		if i == variables.ID {
			continue // Not myself
		}

		// ReceiveSockets initialization to get information from other servers
		ReceiveSockets[i], err = Context.NewSocket(zmq4.REP)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}
		var receiveAddr string
		if !variables.Remote {
			receiveAddr = config.GetRepAddressLocal(i)
		} else {
			receiveAddr = config.GetRepAddress(i)
		}
		err = ReceiveSockets[i].Bind(receiveAddr)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}
		logger.OutLogger.Println("Receive from Server", i, "on", receiveAddr)

		// SendSockets initialization to send information to other servers
		SendSockets[i], err = Context.NewSocket(zmq4.REQ)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}
		var sendAddr string
		if !variables.Remote {
			sendAddr = config.GetReqAddressLocal(i)
		} else {
			sendAddr = config.GetReqAddress(i)
		}
		err = SendSockets[i].Connect(sendAddr)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}
		logger.OutLogger.Println("Send to Server", i, "on", sendAddr)

		// Init message channel
		MessageChannel[i] = make(chan types.Message)
	}

	logger.OutLogger.Println("-----------------------------------------")

	// Initialization of a socket pair to communicate with each one of the clients
	ServerSockets = make(map[int]*zmq4.Socket, variables.Clients)
	ResponseSockets = make(map[int]*zmq4.Socket, variables.Clients)
	for i := 0; i < variables.Clients; i++ {

		// ServerSockets initialization to get clients requests
		ServerSockets[i], err = Context.NewSocket(zmq4.REP)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}
		var serverAddr string
		if !variables.Remote {
			serverAddr = config.GetServerAddressLocal(i)
		} else {
			serverAddr = config.GetServerAddress(i)
		}
		err = ServerSockets[i].Bind(serverAddr)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}
		logger.OutLogger.Println("Requests from Client", i, "on", serverAddr)

		// ResponseSockets initialization to publish the response back to the clients
		ResponseSockets[i], err = Context.NewSocket(zmq4.PUB)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}
		var responseAddr string
		if !variables.Remote {
			responseAddr = config.GetResponseAddressLocal(i)
		} else {
			responseAddr = config.GetResponseAddress(i)
		}
		err = ResponseSockets[i].Bind(responseAddr)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}
		logger.OutLogger.Println("Response to Client", i, "on", responseAddr)
	}

	initRBChannels()

	logger.OutLogger.Print("-----------------------------------------\n\n")
}

// Initializes the RB channels
func initRBChannels() {
	RbChannel["MVC"] = make(map[int]chan struct {
		RbMessage types.RbMessage
		From      int
	})

	RbChannel["VC"] = make(map[int]chan struct {
		RbMessage types.RbMessage
		From      int
	})
}

// Function to modify BC messages if byzantine
func modifyMessageBC(message types.Message, receiver int) types.Message {
	msg := new(types.BcMessage)
	buf := bytes.NewBuffer(message.Payload)
	dec := gob.NewDecoder(buf)
	err := dec.Decode(&msg)
	if err != nil {
		logger.ErrLogger.Fatal(err)
	}

	switch msg.Tag % 3 {
	case 0:
		msg.Value = uint(receiver % 2)
	case 1:
		msg.Value = uint(0)
	case 2:
		msg.Value = uint(1)
	}

	logger.ErrLogger.Print(config.Scenario, ": (", message.Type, ") ", receiver, " --> [",
		msg.Tag, ",", msg.Value, "]\n")

	w := new(bytes.Buffer)
	encoder := gob.NewEncoder(w)
	err = encoder.Encode(msg)
	if err != nil {
		logger.ErrLogger.Fatal(err)
	}

	return types.NewMessage(w.Bytes(), message.Type)
}

// Function to modify messages if byzantine
// halfScenario = true -> send only to the half the right message
// halfScenario = false -> send same value to everyone
func modifyMessage(message types.Message, receiver int, halfScenario bool) types.Message {
	var newPayload []byte

	valueToSend := "0"
	if halfScenario {
		valueToSend = strconv.Itoa(receiver % 2)
	}

	if (message.Type == "BVB") || (message.Type == "BC") {
		msg := new(types.BcMessage)
		buf := bytes.NewBuffer(message.Payload)
		dec := gob.NewDecoder(buf)
		err := dec.Decode(&msg)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}

		val,_ := strconv.Atoi(valueToSend)
		msg.Value = uint(val)

		logger.ErrLogger.Print(config.Scenario, ": (", message.Type, ") ", receiver, " --> [",
			msg.Tag, ",", msg.Value, "]\n")

		w := new(bytes.Buffer)
		encoder := gob.NewEncoder(w)
		err = encoder.Encode(msg)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}
		newPayload = w.Bytes()

	} else if (message.Type == "RB") || (message.Type == "RB_ABC") {
		msg1 := new(types.RbMessage)
		buf1 := bytes.NewBuffer(message.Payload)
		dec1 := gob.NewDecoder(buf1)
		err := dec1.Decode(&msg1)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}

		msg := new(types.Message)
		buf := bytes.NewBuffer(msg1.Value)
		dec := gob.NewDecoder(buf)
		err = dec.Decode(&msg)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}

		var tempPayload []byte

		switch msg.Type {
		case "MVC":
			m := new(types.MvcMessage)
			buf := bytes.NewBuffer(msg.Payload)
			dec := gob.NewDecoder(buf)
			err := dec.Decode(&m)
			if err != nil {
				logger.ErrLogger.Fatal(err)
			}

			m.Value = []byte(valueToSend)

			logger.ErrLogger.Print(config.Scenario, ": (", msg.Type, ") ", receiver, " --> [",
				m.Cid, ",", m.Value, "]\n")

			w := new(bytes.Buffer)
			encoder := gob.NewEncoder(w)
			err = encoder.Encode(m)
			if err != nil {
				logger.ErrLogger.Fatal(err)
			}
			tempPayload = w.Bytes()

		case "VC":
			m := new(types.VcMessage)
			buf := bytes.NewBuffer(msg.Payload)
			dec := gob.NewDecoder(buf)
			err := dec.Decode(&m)
			if err != nil {
				logger.ErrLogger.Fatal(err)
			}

			m.Value = []byte(valueToSend)

			logger.ErrLogger.Print(config.Scenario, ": (", msg.Type, ") ", receiver, " --> [",
				m.Vcid, ",", m.Value, "]\n")

			w := new(bytes.Buffer)
			encoder := gob.NewEncoder(w)
			err = encoder.Encode(m)
			if err != nil {
				logger.ErrLogger.Fatal(err)
			}
			tempPayload = w.Bytes()

		case "ABC":
			m := new(types.AbcMessage)
			buf := bytes.NewBuffer(msg.Payload)
			dec := gob.NewDecoder(buf)
			err := dec.Decode(&m)
			if err != nil {
				logger.ErrLogger.Fatal(err)
			}

			m.Value = []byte(valueToSend)

			logger.ErrLogger.Print(config.Scenario, ": (", msg.Type, ") ", receiver, " --> [",
				m.Num, ",", m.Value, "]\n")

			w := new(bytes.Buffer)
			encoder := gob.NewEncoder(w)
			err = encoder.Encode(m)
			if err != nil {
				logger.ErrLogger.Fatal(err)
			}
			tempPayload = w.Bytes()
		}

		temp := types.NewMessage(tempPayload, msg.Type)

		w := new(bytes.Buffer)
		encoder := gob.NewEncoder(w)
		err = encoder.Encode(temp)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}
		msg1.Value = w.Bytes()

		w1 := new(bytes.Buffer)
		encoder1 := gob.NewEncoder(w1)
		err = encoder1.Encode(msg1)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}
		newPayload = w1.Bytes()

	} else if (message.Type == "SSVC") {

		// encode SSVC message
		msg := new(types.SSVCMessage)
		buf := bytes.NewBuffer(message.Payload)
		dec := gob.NewDecoder(buf)
		err := dec.Decode(&msg)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}

		// change values to send
		for _,t := range []string{"init", "echo", "ready"} {
			for i,_ := range msg.Content[t]{
				msg.Content[t][i].Value = []byte(valueToSend)
			}
		}

		logger.ErrLogger.Print(config.Scenario, ": (", message.Type, ") ", receiver,
			" every value sent --> [", valueToSend, "]\n")

		// encode new message
		w := new(bytes.Buffer)
		encoder := gob.NewEncoder(w)
		err = encoder.Encode(msg)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}
		newPayload = w.Bytes()
	} else if (message.Type == "SSABC") {

		/// encode SSABC message
		msg := new(types.SSABCMessage)
		buf := bytes.NewBuffer(message.Payload)
		dec := gob.NewDecoder(buf)
		err := dec.Decode(&msg)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}

		// change values to send
		for _,t := range []string{"init", "echo", "ready"} {
			for i,_ := range msg.Content[t]{
				msg.Content[t][i].Value = []byte(valueToSend)
				msg.Content[t][i].Num = math.MaxUint32/2
			}
		}

		logger.ErrLogger.Print(config.Scenario, ": (", message.Type, ") ", receiver,
			" every value sent --> [", valueToSend, "]\n")

		// encode new message
		w := new(bytes.Buffer)
		encoder := gob.NewEncoder(w)
		err = encoder.Encode(msg)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}
		newPayload = w.Bytes()
	}

	return types.NewMessage(newPayload, message.Type)
}

// Broadcast - Broadcasts a message to all other servers
func Broadcast(message types.Message) {

	if (config.Scenario == "IDLE") && (variables.Byzantine) {return}

	halfScenario := config.Scenario == "HALF_&_HALF"

	for i := 0; i < variables.N; i++ {
		if i == variables.ID {
			continue // Not myself
		}

		if message.Type != "SSVCDS"{
			// Modify message before sending it, in case of a special scenario
			if (config.Scenario == "BC_ATTACK") && (variables.Byzantine) &&
					((message.Type == "BVB") || (message.Type == "BC")) {
				message = modifyMessageBC(message, i)
			} else if (config.Scenario == "HALF_&_HALF" || config.Scenario == "BZ_ALL") &&
					(variables.Byzantine) {
				message = modifyMessage(message, i, halfScenario)
			}
		}

		timeout := time.NewTicker(10 * time.Second)
		select {
		case MessageChannel[i] <- message:
		case <-timeout.C:
		}
	}
}

// TransmitMessages - Transmits the messages to the other servers [started from main]
func TransmitMessages() {
	for i := 0; i < variables.N; i++ {
		if i == variables.ID {
			continue // Not myself
		}
		go func(i int) { // Initializes them with a goroutine and waits forever
			for message := range MessageChannel[i] {
				w := new(bytes.Buffer)
				encoder := gob.NewEncoder(w)
				err := encoder.Encode(message)
				if err != nil {
					logger.ErrLogger.Fatal(err)
				}

				_, err = SendSockets[i].SendBytes(w.Bytes(), 0)
				if err != nil {
					logger.ErrLogger.Fatal(err)
				}

				_, err = SendSockets[i].Recv(0)
				if err != nil {
					logger.ErrLogger.Fatal(err)
				}
				logger.OutLogger.Println("SENT", message.Type, "to", i)
				variables.MsgMutex.Lock()
				variables.MsgComplexity++
				variables.MsgSize += int64(len(w.Bytes()))
				variables.MsgMutex.Unlock()
			}
		}(i)
	}
}

// Subscribe - Handles the inputs from both clients and other servers [started from main]
func Subscribe() {
	// Gets messages from other servers and handles them
	for i := 0; i < variables.N; i++ {
		if i == variables.ID {
			continue // Not myself
		}
		go func(i int) { // Initializes them with a goroutine and waits forever
			for {
				message, err := ReceiveSockets[i].RecvBytes(0)
				if err != nil {
					logger.ErrLogger.Fatal(err)
				}

				go HandleMessage(message)

				_, err = ReceiveSockets[i].Send("", 0)
				if err != nil {
					logger.ErrLogger.Fatal(err)
				}
			}
		}(i)
	}

	// Gets requests from clients and handles them
	for i := 0; i < variables.Clients; i++ {
		go func(i int) { // Initialize them with a goroutine and waits forever
			for {
				message, err := ServerSockets[i].RecvBytes(0)
				if err != nil {
					logger.ErrLogger.Fatal(err)
				}

				go handleRequest(message, i)

				_, err = ServerSockets[i].Send("", 0)
				if err != nil {
					logger.ErrLogger.Fatal(err)
				}
			}
		}(i)
	}
}

// Put client's message in RequestChannel to be handled
func handleRequest(message []byte, from int) {
	logger.OutLogger.Println("RECEIVED REQ from", from)
	RequestChannel <- message
}

// HandleMessage - Handles the messages from the other servers								SS
func HandleMessage(msg []byte) {
	message := new(types.Message)
	buffer := bytes.NewBuffer([]byte(msg))
	decoder := gob.NewDecoder(buffer)
	err := decoder.Decode(&message)
	if err != nil {
		logger.ErrLogger.Fatal(err)
	}

	if !(threshenc.VerifyMessage(message.Payload, message.Signature, message.From)) {
		logger.OutLogger.Println("INVALID", message.Type, "from", message.From)
		return
	}

	logger.OutLogger.Println("RECEIVED", message.Type, "from", message.From)

	switch message.Type {
	case "BVB":
		bcMessage := new(types.BcMessage)
		buf := bytes.NewBuffer(message.Payload)
		dec := gob.NewDecoder(buf)
		err = dec.Decode(&bcMessage)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}

		tag := bcMessage.Tag
		BvbMutex.Lock()
		if _, in := BvbChannel[tag]; !in {
			BvbChannel[tag] = make(chan struct {
				BcMessage types.BcMessage
				From      int
			})
		}
		msgChannel := BvbChannel[tag]
		BvbMutex.Unlock()

		msgChannel <- struct {
			BcMessage types.BcMessage
			From      int
		}{BcMessage: *bcMessage, From: message.From}

	case "BC":
		bcMessage := new(types.BcMessage)
		buf := bytes.NewBuffer(message.Payload)
		dec := gob.NewDecoder(buf)
		err = dec.Decode(&bcMessage)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}

		tag := bcMessage.Tag
		BcMutex.Lock()
		if _, in := BcChannel[tag]; !in {
			BcChannel[tag] = make(chan struct {
				BcMessage types.BcMessage
				From      int
			})
		}
		msgChannel := BcChannel[tag]
		BcMutex.Unlock()

		msgChannel <- struct {
			BcMessage types.BcMessage
			From      int
		}{BcMessage: *bcMessage, From: message.From}

	case "RB":
		rbMessage := new(types.RbMessage)
		buf := bytes.NewBuffer(message.Payload)
		dec := gob.NewDecoder(buf)
		err = dec.Decode(&rbMessage)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}

		rbid := rbMessage.Rbid
		rbType := rbMessage.Type
		RbMutex.Lock()
		if _, in := RbChannel[rbType][rbid]; !in {
			RbChannel[rbType][rbid] = make(chan struct {
				RbMessage types.RbMessage
				From      int
			})
		}
		msgChannel := RbChannel[rbType][rbid]
		RbMutex.Unlock()

		msgChannel <- struct {
			RbMessage types.RbMessage
			From      int
		}{RbMessage: *rbMessage, From: message.From}

	case "RB_ABC":
		rbMessage := new(types.RbMessage)
		buf := bytes.NewBuffer(message.Payload)
		dec := gob.NewDecoder(buf)
		err = dec.Decode(&rbMessage)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}

		RbAbcChannel <- struct {
			RbMessage types.RbMessage
			From      int
		}{RbMessage: *rbMessage, From: message.From}

	case "MVC":
		mvcMessage := new(types.MvcMessage)
		buf := bytes.NewBuffer(message.Payload)
		dec := gob.NewDecoder(buf)
		err = dec.Decode(&mvcMessage)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}

		cid := mvcMessage.Cid
		MvcMutex.Lock()
		if _, in := MvcChannel[cid]; !in {
			MvcChannel[cid] = make(chan struct {
				MvcMessage types.MvcMessage
				From       int
			})
		}
		msgChannel := MvcChannel[cid]
		MvcMutex.Unlock()

		msgChannel <- struct {
			MvcMessage types.MvcMessage
			From       int
		}{MvcMessage: *mvcMessage, From: message.From}

	case "VC":
		vcMessage := new(types.VcMessage)
		buf := bytes.NewBuffer(message.Payload)
		dec := gob.NewDecoder(buf)
		err = dec.Decode(&vcMessage)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}

		vcid := vcMessage.Vcid
		VcMutex.Lock()
		if _, in := VcChannel[vcid]; !in {
			VcChannel[vcid] = make(chan struct {
				VcMessage types.VcMessage
				From      int
			})
		}
		msgChannel := VcChannel[vcid]
		VcMutex.Unlock()

		msgChannel <- struct {
			VcMessage types.VcMessage
			From      int
		}{VcMessage: *vcMessage, From: message.From}

	case "ABC":
		abcMessage := new(types.AbcMessage)
		buf := bytes.NewBuffer(message.Payload)
		dec := gob.NewDecoder(buf)
		err = dec.Decode(&abcMessage)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}

		AbcChannel <- struct {
			AbcMessage types.AbcMessage
			From       int
		}{AbcMessage: *abcMessage, From: message.From}

	case "SSVC":
		ssvcMessage := new(types.SSVCMessage)
		buf := bytes.NewBuffer(message.Payload)
		dec := gob.NewDecoder(buf)
		err = dec.Decode(&ssvcMessage)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}

		ssvcid := ssvcMessage.SSVCid
		SSVCMutex.Lock()
		if _, in := SSVCChannel[ssvcid]; !in {
			SSVCChannel[ssvcid] = make(chan struct {
				SSVCMessage types.SSVCMessage
				From      int
			})
		}
		msgChannel := SSVCChannel[ssvcid]
		SSVCMutex.Unlock()

		msgChannel <- struct {
			SSVCMessage types.SSVCMessage
			From      int
		}{SSVCMessage: *ssvcMessage, From: message.From}

	case "SSVCDS": // SSVC decision
		type ssvcDecision struct{
			Vector	map[int][]byte
			SSVCid	int
		}
		vectorMessage := new(ssvcDecision)
		buf := bytes.NewBuffer(message.Payload)
		dec := gob.NewDecoder(buf)
		err = dec.Decode(&vectorMessage)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}

		ssvcid := vectorMessage.SSVCid
		SSVCDecisionsMutex.Lock()
		if _, in := SSVCDecisionsChannel[ssvcid]; !in {
			SSVCDecisionsChannel[ssvcid] = make(chan struct {
				Vector map[int][]byte
				From      int
			})
		}
		msgChannel := SSVCDecisionsChannel[ssvcid]
		SSVCDecisionsMutex.Unlock()

		 msgChannel <- struct {
			Vector map[int][]byte
			From      int
		}{Vector: vectorMessage.Vector, From: message.From}

	case "SSABC":
		ssabcMessage := new(types.SSABCMessage)
		buf := bytes.NewBuffer(message.Payload)
		dec := gob.NewDecoder(buf)
		err = dec.Decode(&ssabcMessage)
		if err != nil {
			logger.ErrLogger.Fatal(err)
		}

		SSABCChannel <- struct {
			SSABCMessage types.SSABCMessage
			From      int
		}{SSABCMessage: *ssabcMessage, From: message.From}

	} // switch end
}

// ReplyClient - Sends back a response to the client
func ReplyClient(reply types.Reply, to int) {
	w := new(bytes.Buffer)
	encoder := gob.NewEncoder(w)
	err := encoder.Encode(reply)
	if err != nil {
		logger.ErrLogger.Fatal(err)
	}

	_, err = ResponseSockets[to].SendBytes(w.Bytes(), 0)
	if err != nil {
		logger.ErrLogger.Fatal(err)
	}
	logger.OutLogger.Println("REPLIED Client", to, "-", reply.Value)

	variables.MsgMutex.Lock()
	variables.MsgComplexity++
	variables.MsgSize += int64(len(w.Bytes()))
	variables.MsgMutex.Unlock()
}
