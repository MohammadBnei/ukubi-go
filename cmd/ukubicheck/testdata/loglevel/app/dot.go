package app

import . "log"

func g() { Println("x") } // want `^log\.Println:`
