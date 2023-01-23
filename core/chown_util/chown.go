package chown_util

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"os"
	"os/user"
	"strconv"
)

var (
	currentUser *userIds
	realUser    *userIds
)

type userIds struct {
	username string
	homedir  string
	uid      int
	gid      int
}

func populateUsers() error {
	if currentUser == nil {
		u, err := user.Current()
		if err != nil {
			return err
		}
		if u == nil {
			return errors.New("couldnt get current user")
		}
		currentUser, err = convertUser(u)
		if err != nil {
			return err
		}
	}
	if currentUser.uid != 0 {
		return nil
	}

	sudoUserName := os.Getenv("SUDO_USER")
	if sudoUserName == "" {
		sudoUserName = os.Getenv("USER")
	}
	if sudoUserName == "" || sudoUserName == currentUser.username {
		return errors.New("couldnt get real user")
	}

	sudoUser, err := user.Lookup(sudoUserName)
	if err != nil {
		return errors.Wrap(err, "error getting real user")
	}
	if sudoUser == nil {
		return errors.New("returned real user is nil")
	}
	realUser, err = convertUser(sudoUser)
	if err != nil {
		return errors.Wrap(err, "error converting real user")
	}
	return nil
}

func convertUser(u *user.User) (*userIds, error) {
	userId, err := strconv.Atoi(u.Uid)
	if err != nil {
		return nil, err
	}
	groupId, err := strconv.Atoi(u.Gid)
	if err != nil {
		return nil, err
	}
	return &userIds{
		username: u.Username,
		uid:      userId,
		gid:      groupId,
		homedir:  u.HomeDir,
	}, nil
}

// TryRectifyRootFiles tries to change the ownership of the files given to the "real" current user
// this is useful when people have to run the Barbe command as root, but want to keep the file ownership
func TryRectifyRootFiles(ctx context.Context, filePaths []string) {
	if len(filePaths) == 0 {
		return
	}

	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("error rectifying root files: %v", err)
			log.Warn().Err(errors.New("panic in TryRectifyRootFiles")).Msg("")
		}
	}()

	err := populateUsers()
	if err != nil {
		log.Ctx(ctx).Debug().Err(err).Msg("error populating users for root files rectification")
		return
	}
	if realUser == nil {
		log.Ctx(ctx).Debug().Msg("real user is nil, cant rectify root files")
		return
	}

	for _, filePath := range filePaths {
		err := os.Chown(filePath, realUser.uid, realUser.gid)
		if err != nil {
			log.Ctx(ctx).Debug().Err(err).Msg("error changing owner of file '" + filePath + "'")
		}
	}
	return
}

//the aws sdk relies on the "HOME" env var to find the .aws/credentials file,
//when executing locally with sudo, the HOME env var is set to the root user's home dir
//which most likely doesn't have the .aws/credentials file,
//so we adjust it to the current non-sudo user's home dir
func TryAdjustRootHomeDir(ctx context.Context) {
	err := populateUsers()
	if err != nil {
		log.Ctx(ctx).Debug().Err(err).Msg("error populating users for home adjustment")
		return
	}
	if realUser == nil || realUser.homedir == "" {
		return
	}
	err = os.Setenv("HOME", realUser.homedir)
	if err != nil {
		log.Ctx(ctx).Debug().Err(err).Msg("error setting home dir")
	}
}

func GetSudoerUser() (uid int, gid int, e error) {
	err := populateUsers()
	if err != nil {
		return -1, -1, err
	}
	if realUser == nil {
		return -1, -1, nil
	}
	return realUser.uid, realUser.gid, nil
}
